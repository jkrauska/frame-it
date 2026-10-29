package gallery

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jkrauska/frame-it/internal/userlog"
)

//go:embed page.html
var pageFS embed.FS

var pageTmpl = template.Must(template.ParseFS(pageFS, "page.html"))

// DefaultPort is the port the gallery listens on when none is given.
const DefaultPort = 8787

// DefaultBind is the interface the gallery listens on when none is given. It is
// reachable from the LAN so the grid can be opened on a phone or laptop; the
// server is read-only and unauthenticated, so pass 127.0.0.1 to keep it local.
const DefaultBind = "0.0.0.0"

// Options configures the gallery server.
type Options struct {
	Dir   string // archive directory to read
	Limit int    // number of recent images to show
	Bind  string // interface to listen on (default 0.0.0.0)
	Port  int
	Loc   *time.Location
}

type server struct {
	opts   Options
	thumbs *thumbCache
}

// Handler serves the gallery: the grid at /, thumbnails at /thumb/<name>, and
// originals at /full/<name>. The listing is rebuilt per request so a reload
// picks up images archived while the server is running.
func Handler(opts Options) http.Handler {
	return newServer(opts).mux()
}

func newServer(opts Options) *server {
	if opts.Limit <= 0 {
		opts.Limit = DefaultLimit
	}
	if opts.Loc == nil {
		opts.Loc = time.Local
	}
	return &server{opts: opts, thumbs: newThumbCache()}
}

func (s *server) mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/thumb/", s.thumb)
	mux.HandleFunc("/full/", s.full)
	return mux
}

// warm renders the thumbnails for the current listing ahead of the first page
// view, so the grid isn't a wall of empty frames while 4K originals decode.
func (s *server) warm(ctx context.Context) {
	entries, err := List(s.opts.Dir, s.opts.Limit, s.opts.Loc)
	if err != nil {
		return
	}

	const workers = 4
	paths := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range paths {
				if ctx.Err() != nil {
					return
				}
				_, _ = s.thumbs.Get(path)
			}
		}()
	}
	for _, e := range entries {
		select {
		case <-ctx.Done():
		case paths <- e.Path:
			continue
		}
		break
	}
	close(paths)
	wg.Wait()
}

// Serve runs the gallery on loopback until ctx is cancelled.
func Serve(ctx context.Context, opts Options, log *userlog.Logger) error {
	if opts.Port <= 0 {
		opts.Port = DefaultPort
	}
	if opts.Bind == "" {
		opts.Bind = DefaultBind
	}

	addr := net.JoinHostPort(opts.Bind, strconv.Itoa(opts.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	gs := newServer(opts)
	srv := &http.Server{
		Handler:           gs.mux(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go gs.warm(ctx)

	log.Step(fmt.Sprintf("Gallery running at %s (Ctrl+C to stop)", LocalURL(opts.Port)))
	log.Note(fmt.Sprintf("Showing the %d newest images in %s", opts.Limit, opts.Dir))
	if opts.Bind != "127.0.0.1" && opts.Bind != "localhost" {
		if ip := lanIP(); ip != "" {
			log.Note(fmt.Sprintf("Reachable on this network at http://%s:%d (no password — anyone on the LAN can browse it)", ip, opts.Port))
		} else {
			log.Note(fmt.Sprintf("Listening on %s — no password, so anyone who can reach this host can browse it", addr))
		}
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return ctx.Err()
	}
}

// LocalURL returns the loopback address of the gallery, used for --open.
func LocalURL(port int) string {
	if port <= 0 {
		port = DefaultPort
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// lanIP returns this host's primary non-loopback IPv4 address, or "" if there
// isn't one. It is only used to print a URL that works from other devices.
func lanIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	entries, err := List(s.opts.Dir, s.opts.Limit, s.opts.Loc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Dir       string
		Entries   []Entry
		Generated string
	}{
		Dir:       s.opts.Dir,
		Entries:   entries,
		Generated: time.Now().In(s.opts.Loc).Format("Mon Jan 2, 3:04pm"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := pageTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) thumb(w http.ResponseWriter, r *http.Request) {
	path, ok := s.resolve(w, r, "/thumb/")
	if !ok {
		return
	}

	data, err := s.thumbs.Get(path)
	if err != nil {
		http.Error(w, "could not render thumbnail", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "thumb.jpg", time.Time{}, bytes.NewReader(data))
}

func (s *server) full(w http.ResponseWriter, r *http.Request) {
	path, ok := s.resolve(w, r, "/full/")
	if !ok {
		return
	}
	http.ServeFile(w, r, path)
}

// resolve maps a request path to a file inside the archive directory, rejecting
// anything that isn't a plain image filename living directly in that directory.
func (s *server) resolve(w http.ResponseWriter, r *http.Request, prefix string) (string, bool) {
	name := strings.TrimPrefix(r.URL.Path, prefix)
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") || !isImage(name) {
		http.NotFound(w, r)
		return "", false
	}
	return filepath.Join(s.opts.Dir, name), true
}
