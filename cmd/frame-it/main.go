package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jkrauska/frame-it/internal/config"
	"github.com/jkrauska/frame-it/internal/discover"
	"github.com/jkrauska/frame-it/internal/overlay"
	"github.com/jkrauska/frame-it/internal/samsung"
	"github.com/jkrauska/frame-it/internal/userlog"
	"github.com/jkrauska/frame-it/internal/wallpaper"
)

const usage = `frame-it — upload images to a Samsung Frame TV

Usage:
  frame-it discover                Find Samsung TVs on the LAN (mDNS + SSDP)
  frame-it pair                    Authorize with the TV (accept on-screen prompt)
  frame-it upload <image>...       Upload image(s) and display the last one
  frame-it list                    List uploaded images on the TV
  frame-it select <content-id>     Display an existing image
  frame-it delete <content-id>...  Delete image(s) from the TV
  frame-it wallpaper [query]       Fetch wallpaper art and upload to TV
  frame-it schedule [query]        Refresh wallpaper on a daily interval (7am–9pm default)

Options:
  --host IP          TV IP (optional: saved config, FRAME_IT_HOST, or auto-discover)
  --token-dir PATH   Token and config directory (default: ~/.frame-it)
  --name NAME        Client name shown on TV prompt (default: frame-it)
  --matte ID         Matte style (default: none; e.g. shadowbox_polar, modern_apricot)
  --no-matte         Upload without a matte frame (same as --matte none)
  --show             After upload, display the image (default: true)
  --no-show          Upload without switching the displayed image
  --no-date          Skip date stamp on uploaded images
  --mdns-only        Discover using mDNS only (no SSDP)
  --ssdp-only        Discover using SSDP only (no mDNS)
  -v                 Verbose logging

Wallpaper options (wallpaper command):
  --source NAME      wallhaven (default), unsplash, or pixabay
  --api-key KEY      API key for the selected source (see env vars below)
  --id ID            Use a specific wallpaper/photo ID
  --sort MODE        wallhaven: random (default), toplist, …; pixabay: popular, latest
  --save PATH        Also save the downloaded image locally
  --download-only    Download only; do not upload to the TV
  --no-replace       Keep previous wallpaper uploads (default: rotate two slots)

Schedule options (schedule command):
  --interval DUR     Time between refreshes (default: 15m)
  --start TIME       Daily start time (default: 7:00)
  --end TIME         Daily end time, exclusive (default: 21:00)
  --timezone NAME    IANA timezone (default: local)
  --once             Run one refresh if inside the window, then exit
  --no-run-on-start  Wait for the first interval before refreshing

Environment:
  WALLHAVEN_API_KEY     Wallhaven key (optional for SFW)
  UNSPLASH_ACCESS_KEY   Unsplash access key (required for --source unsplash)
  PIXABAY_API_KEY       Pixabay key (required for --source pixabay)

Examples:
  frame-it discover
  frame-it pair
  frame-it upload ./photos/*.jpg
  frame-it --host 192.168.1.50 list
  frame-it wallpaper nature
  frame-it wallpaper --source unsplash mountains
  frame-it wallpaper --source pixabay --download-only --save ./art.jpg
  frame-it wallpaper --source wallhaven --id 94x38z
  frame-it schedule mountains
  frame-it schedule --interval 15m --start 7:00 --end 21:00 nature
`

var errHelp = errors.New("help requested")

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		return 1
	}

	cmd := os.Args[1]
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return 0
	}

	flags, args, err := parseFlags(os.Args[2:])
	if err != nil {
		if errors.Is(err, errHelp) {
			fmt.Print(usage)
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	log := userlog.New(os.Stderr, flags.verbose)

	ctx := context.Background()

	if cmd == "discover" {
		return runDiscover(ctx, flags, log)
	}

	if cmd == "wallpaper" {
		return runWallpaper(ctx, flags, args, log)
	}

	if cmd == "schedule" {
		return runSchedule(ctx, flags, args, log)
	}

	host, err := resolveHost(ctx, flags, log)
	if err != nil {
		log.Error(err.Error())
		return 1
	}

	client := samsung.NewClient(samsung.Options{
		Host:          host,
		ClientName:    flags.name,
		TokenDir:      flags.tokenDir,
		Matte:         flags.matte,
		Timeout:       30 * time.Second,
		SkipTLSVerify: true,
		Logger:        log.Slog(),
	})

	switch cmd {
	case "pair":
		if err := client.Pair(ctx); err != nil {
			log.Error(fmt.Sprintf("Pairing failed: %v", err))
			return 1
		}
		log.Step("Paired successfully — you're all set")
		return 0

	case "upload":
		if len(args) == 0 {
			log.Error("Upload needs at least one image path")
			return 1
		}
		return uploadImages(ctx, client, args, flags.show, flags.stampDate, log)

	case "list":
		if err := client.Connect(ctx); err != nil {
			log.Error(fmt.Sprintf("Could not connect: %v", err))
			return 1
		}
		defer func() { _ = client.Close() }()

		items, err := client.ListUploaded(ctx)
		if err != nil {
			log.Error(fmt.Sprintf("Could not list images: %v", err))
			return 1
		}
		if len(items) == 0 {
			log.Step("No uploaded images on the TV yet")
			return 0
		}
		log.Step(fmt.Sprintf("%d image(s) on the TV:", len(items)))
		for _, item := range items {
			log.Note(item.ContentID)
		}
		return 0

	case "select":
		if len(args) != 1 {
			log.Error("Select needs exactly one content-id")
			return 1
		}
		if err := client.Connect(ctx); err != nil {
			log.Error(fmt.Sprintf("Could not connect: %v", err))
			return 1
		}
		defer func() { _ = client.Close() }()

		if err := client.SelectImage(ctx, args[0]); err != nil {
			log.Error(fmt.Sprintf("Could not switch image: %v", err))
			return 1
		}
		log.Step("Now showing on the TV")
		return 0

	case "delete":
		if len(args) == 0 {
			log.Error("Delete needs at least one content-id")
			return 1
		}
		if err := client.Connect(ctx); err != nil {
			log.Error(fmt.Sprintf("Could not connect: %v", err))
			return 1
		}
		defer func() { _ = client.Close() }()

		if err := client.DeleteImages(ctx, args); err != nil {
			log.Error(fmt.Sprintf("Could not delete: %v", err))
			return 1
		}
		log.Step(fmt.Sprintf("Deleted %d image(s) from the TV", len(args)))
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		fmt.Print(usage)
		return 1
	}
}

func uploadImages(ctx context.Context, client *samsung.Client, paths []string, show, stampDate bool, log *userlog.Logger) int {
	return uploadImagesWithReplace(ctx, client, paths, show, stampDate, "", log, "", false)
}

func uploadImagesWithReplace(ctx context.Context, client *samsung.Client, paths []string, show, stampDate bool, caption string, log *userlog.Logger, tokenDir string, replacePrevious bool) int {
	if err := client.Connect(ctx); err != nil {
		log.Error(fmt.Sprintf("Could not connect: %v", err))
		return 1
	}
	defer func() { _ = client.Close() }()

	var wallpaperSlot int
	var replaceID string
	if replacePrevious && tokenDir != "" {
		slots, err := config.LoadWallpaperSlots(tokenDir)
		if err != nil {
			log.Error(fmt.Sprintf("Could not load config: %v", err))
			return 1
		}
		targetSlot, oldID := config.NextWallpaperTarget(slots)
		wallpaperSlot = targetSlot
		replaceID = oldID
	}

	var lastID string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			log.Warn(fmt.Sprintf("Skipping %s — only JPG and PNG are supported", path))
			continue
		}

		uploadLabel := filepath.Base(path)
		if wallpaperSlot != 0 {
			uploadLabel = config.WallpaperSlotLabel(wallpaperSlot)
		}

		if stampDate {
			log.Step("Preparing 4K image with date stamp…")
		} else if strings.TrimSpace(caption) != "" {
			log.Step("Preparing 4K image with caption…")
		} else {
			log.Step("Preparing 4K image…")
		}
		uploadPath, cleanup, err := overlay.PrepareUploadPath(path, overlay.PrepareOptions{
			StampDate: stampDate,
			When:      time.Now(),
			Caption:   caption,
		})
		if err != nil {
			log.Error(fmt.Sprintf("Could not prepare image %s: %v", path, err))
			return 1
		}

		log.Step(fmt.Sprintf("Uploading to %s…", uploadLabel))
		contentID, err := client.Upload(ctx, uploadPath)
		cleanup()
		if err != nil {
			log.Error(fmt.Sprintf("Upload failed: %v", err))
			return 1
		}
		if wallpaperSlot != 0 {
			log.Note(fmt.Sprintf("TV assigned %s to %s", contentID, config.WallpaperSlotLabel(wallpaperSlot)))
		} else {
			log.Note(fmt.Sprintf("Saved on TV as %s", contentID))
		}
		lastID = contentID
	}

	if show && lastID != "" {
		if err := client.SelectImage(ctx, lastID); err != nil {
			log.Error(fmt.Sprintf("Could not show image: %v", err))
			return 1
		}
		if wallpaperSlot != 0 {
			log.Step(fmt.Sprintf("Now showing %s on the TV", config.WallpaperSlotLabel(wallpaperSlot)))
		} else {
			log.Step("Now showing on the TV")
		}
	}

	if replaceID != "" {
		slotLabel := config.WallpaperSlotLabel(wallpaperSlot)
		log.Step(fmt.Sprintf("Removing replaced %s image %s…", slotLabel, replaceID))
		if err := client.DeleteImages(ctx, []string{replaceID}); err != nil {
			log.Warn(fmt.Sprintf("Could not delete replaced wallpaper: %v", err))
		}
	}

	if replacePrevious && tokenDir != "" && lastID != "" && wallpaperSlot != 0 {
		if err := config.SetWallpaperSlot(tokenDir, wallpaperSlot, lastID); err != nil {
			log.Warn(fmt.Sprintf("Could not save wallpaper slot to config: %v", err))
		}
	}

	return 0
}

type cliFlags struct {
	host           string
	tokenDir       string
	name           string
	matte          string
	show           bool
	stampDate      bool
	verbose        bool
	mdnsOnly       bool
	ssdpOnly       bool
	wallhavenKey    string
	unsplashKey     string
	pixabayKey      string
	apiKey          string
	wallpaperSource string
	wallpaperID     string
	wallpaperSort   string
	wallpaperSave   string
	downloadOnly    bool
	wallpaperReplace bool
	scheduleInterval string
	scheduleStart    string
	scheduleEnd      string
	scheduleTZ       string
	scheduleOnce     bool
	scheduleRunOnStart bool
}

func parseFlags(args []string) (cliFlags, []string, error) {
	f := cliFlags{
		tokenDir:      defaultTokenDir(),
		name:          "frame-it",
		matte:         "none",
		show:             true,
		stampDate:        true,
		wallhavenKey:     os.Getenv("WALLHAVEN_API_KEY"),
		unsplashKey:   os.Getenv("UNSPLASH_ACCESS_KEY"),
		pixabayKey:    os.Getenv("PIXABAY_API_KEY"),
		wallpaperSort:      "random",
		wallpaperReplace:   true,
		scheduleInterval:   "15m",
		scheduleStart:      "7:00",
		scheduleEnd:        "21:00",
		scheduleRunOnStart: true,
	}

	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--host":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--host requires a value")
			}
			i++
			f.host = args[i]
		case "--token-dir":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--token-dir requires a value")
			}
			i++
			f.tokenDir = args[i]
		case "--name":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--name requires a value")
			}
			i++
			f.name = args[i]
		case "--matte":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--matte requires a value")
			}
			i++
			f.matte = args[i]
		case "--no-matte":
			f.matte = "none"
		case "--show":
			f.show = true
		case "--no-show":
			f.show = false
		case "--no-date":
			f.stampDate = false
		case "-v", "--verbose":
			f.verbose = true
		case "--mdns-only":
			f.mdnsOnly = true
		case "--ssdp-only":
			f.ssdpOnly = true
		case "--api-key":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--api-key requires a value")
			}
			i++
			f.apiKey = args[i]
		case "--source":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--source requires a value")
			}
			i++
			f.wallpaperSource = args[i]
		case "--id":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--id requires a value")
			}
			i++
			f.wallpaperID = args[i]
		case "--sort":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--sort requires a value")
			}
			i++
			f.wallpaperSort = args[i]
		case "--save":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--save requires a value")
			}
			i++
			f.wallpaperSave = args[i]
		case "--download-only":
			f.downloadOnly = true
		case "--no-replace":
			f.wallpaperReplace = false
		case "--interval":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--interval requires a value")
			}
			i++
			f.scheduleInterval = args[i]
		case "--start":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--start requires a value")
			}
			i++
			f.scheduleStart = args[i]
		case "--end":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--end requires a value")
			}
			i++
			f.scheduleEnd = args[i]
		case "--timezone":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--timezone requires a value")
			}
			i++
			f.scheduleTZ = args[i]
		case "--once":
			f.scheduleOnce = true
		case "--no-run-on-start":
			f.scheduleRunOnStart = false
		case "-h", "--help":
			return f, nil, errHelp
		default:
			if strings.HasPrefix(arg, "-") {
				return f, nil, fmt.Errorf("unknown flag: %s", arg)
			}
			positional = append(positional, arg)
		}
	}

	if f.mdnsOnly && f.ssdpOnly {
		return f, nil, fmt.Errorf("use either --mdns-only or --ssdp-only, not both")
	}

	return f, positional, nil
}

func defaultTokenDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".frame-it"
	}
	return filepath.Join(home, ".frame-it")
}

func discoverOpts(flags cliFlags) discover.Options {
	opts := discover.DefaultOptions()
	if flags.mdnsOnly {
		opts.SSDP = false
	}
	if flags.ssdpOnly {
		opts.MDNS = false
	}
	return opts
}

func resolveHost(ctx context.Context, flags cliFlags, log *userlog.Logger) (string, error) {
	if flags.host != "" {
		if err := config.SetHost(flags.tokenDir, flags.host); err != nil {
			log.Warn(fmt.Sprintf("Could not save TV host to config: %v", err))
		}
		return flags.host, nil
	}

	if host := os.Getenv("FRAME_IT_HOST"); host != "" {
		return host, nil
	}

	cfg, err := config.Load(flags.tokenDir)
	if err != nil {
		return "", err
	}
	if cfg.Host != "" {
		log.Note("Using TV at " + cfg.Host + " (from config)")
		return cfg.Host, nil
	}

	log.Step("Looking for Samsung TVs on your network…")
	tvs, err := discover.Find(ctx, discoverOpts(flags))
	if err != nil {
		return "", err
	}

	tv, err := discover.PickFrameTV(tvs)
	if err != nil {
		return "", err
	}

	label := tv.IP
	if tv.Name != "" {
		label = fmt.Sprintf("%s (%s)", tv.Name, tv.IP)
	}
	log.Note("Found " + label)

	if err := config.SetHost(flags.tokenDir, tv.IP); err != nil {
		log.Warn(fmt.Sprintf("Could not save TV host to config: %v", err))
	}

	return tv.IP, nil
}

func runDiscover(ctx context.Context, flags cliFlags, log *userlog.Logger) int {
	log.Step("Scanning for Samsung TVs…")
	tvs, err := discover.Find(ctx, discoverOpts(flags))
	if err != nil {
		log.Error(fmt.Sprintf("Scan failed: %v", err))
		return 1
	}

	if len(tvs) == 0 {
		log.Step("No Samsung TVs found")
		return 0
	}

	log.Step(fmt.Sprintf("Found %d TV(s):", len(tvs)))
	for _, tv := range tvs {
		name := tv.Name
		if name == "" {
			name = "Samsung TV"
		}
		model := tv.Model
		if model != "" {
			name = name + " · " + model
		}
		frame := ""
		if tv.FrameTVSupport {
			frame = " · Frame TV"
		}
		host := ""
		if tv.Hostname != "" {
			host = " · " + tv.Hostname
		}
		via := strings.Join(tv.Sources, "+")
		log.Note(fmt.Sprintf("%s — %s%s%s (via %s)", tv.IP, name, frame, host, via))
	}
	return 0
}

func runWallpaper(ctx context.Context, flags cliFlags, args []string, log *userlog.Logger) int {
	query := strings.TrimSpace(strings.Join(args, " "))

	source, err := wallpaper.ParseSource(flags.wallpaperSource)
	if err != nil {
		log.Error(err.Error())
		return 1
	}

	fetchOpts := wallpaperOptions(flags, source, query)

	if flags.wallpaperID != "" {
		log.Step(fmt.Sprintf("Fetching %s from %s…", flags.wallpaperID, source.Label()))
	} else if query != "" {
		log.Step(fmt.Sprintf("Searching %s for %q…", source.Label(), query))
	} else {
		log.Step(fmt.Sprintf("Picking a random wallpaper from %s…", source.Label()))
	}

	img, err := wallpaper.Fetch(ctx, fetchOpts)
	if err != nil {
		log.Error(err.Error())
		return 1
	}

	note := fmt.Sprintf("Selected %s · %s", img.ID, img.Resolution)
	if img.Description != "" {
		note += " · " + img.Description
	}
	if img.Credit != "" {
		note += " · " + img.Credit
	}
	log.Note(note)

	destPath := flags.wallpaperSave
	if destPath == "" {
		tmpFile, err := wallpaper.TempFile(img)
		if err != nil {
			log.Error(fmt.Sprintf("Could not create temp file: %v", err))
			return 1
		}
		destPath = tmpFile.Name()
		_ = tmpFile.Close()
		defer func() { _ = os.Remove(destPath) }()
	}

	log.Step("Downloading image…")
	if err := wallpaper.Download(ctx, img, destPath); err != nil {
		log.Error(fmt.Sprintf("Download failed: %v", err))
		return 1
	}
	if flags.wallpaperSave != "" {
		log.Note("Saved to " + destPath)
	}

	if flags.downloadOnly {
		log.Step("Download complete")
		return 0
	}

	host, err := resolveHost(ctx, flags, log)
	if err != nil {
		log.Error(err.Error())
		return 1
	}

	client := samsung.NewClient(samsung.Options{
		Host:          host,
		ClientName:    flags.name,
		TokenDir:      flags.tokenDir,
		Matte:         flags.matte,
		Timeout:       30 * time.Second,
		SkipTLSVerify: true,
		Logger:        log.Slog(),
	})

	return uploadImagesWithReplace(ctx, client, []string{destPath}, flags.show, flags.stampDate, img.Caption(), log, flags.tokenDir, flags.wallpaperReplace)
}

func wallpaperOptions(flags cliFlags, source wallpaper.Source, query string) wallpaper.Options {
	opts := wallpaper.Options{
		Source:       source,
		Query:        query,
		ID:           flags.wallpaperID,
		Sort:         flags.wallpaperSort,
		WallhavenKey: flags.wallhavenKey,
		UnsplashKey:  flags.unsplashKey,
		PixabayKey:   flags.pixabayKey,
	}
	if flags.apiKey != "" {
		switch source {
		case wallpaper.SourceUnsplash:
			opts.UnsplashKey = flags.apiKey
		case wallpaper.SourcePixabay:
			opts.PixabayKey = flags.apiKey
		default:
			opts.WallhavenKey = flags.apiKey
		}
	}
	return opts
}
