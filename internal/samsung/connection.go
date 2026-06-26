package samsung

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const maxMessageSize = 16 * 1024 * 1024

type connConfig struct {
	host          string
	port          int
	endpoint      string
	name          string
	tokenFile     string
	timeout       time.Duration
	skipTLSVerify bool
	logger        *slog.Logger
}

type connection struct {
	host          string
	port          int
	endpoint      string
	name          string
	tokenFile     string
	timeout       time.Duration
	skipTLSVerify bool
	logger        *slog.Logger

	mu       sync.Mutex
	conn     *websocket.Conn
	closed   atomic.Bool
	recvDone chan struct{}

	pendingMu sync.Mutex
	pending   map[string]chan json.RawMessage
}

func newConnection(cfg connConfig) *connection {
	return &connection{
		host:          cfg.host,
		port:          cfg.port,
		endpoint:      cfg.endpoint,
		name:          cfg.name,
		tokenFile:     cfg.tokenFile,
		timeout:       cfg.timeout,
		skipTLSVerify: cfg.skipTLSVerify,
		logger:        cfg.logger,
		pending:       make(map[string]chan json.RawMessage),
	}
}

func (c *connection) SendAndWait(ctx context.Context, payload []byte, requestID string, timeout time.Duration) (json.RawMessage, error) {
	ch := make(chan json.RawMessage, 1)

	c.pendingMu.Lock()
	c.pending[requestID] = ch
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, requestID)
		c.pendingMu.Unlock()
	}()

	if err := c.SendText(ctx, payload); err != nil {
		return nil, err
	}

	select {
	case data, ok := <-ch:
		if !ok {
			return nil, ErrNotConnected
		}
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("%w: waiting for response %s", ErrTimeout, requestID)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *connection) SendText(ctx context.Context, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return ErrNotConnected
	}

	c.logger.Debug("send text", "bytes", len(payload))
	return c.conn.Write(ctx, websocket.MessageText, payload)
}

func (c *connection) SendBinary(ctx context.Context, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return ErrNotConnected
	}

	c.logger.Debug("send binary image", "bytes", len(payload))
	return c.conn.Write(ctx, websocket.MessageBinary, payload)
}

func (c *connection) formatURL(token string) string {
	b64Name := base64.StdEncoding.EncodeToString([]byte(c.name))
	u := url.URL{
		Scheme: "wss",
		Host:   fmt.Sprintf("%s:%d", c.host, c.port),
		Path:   fmt.Sprintf("/api/v2/channels/%s", c.endpoint),
	}
	q := u.Query()
	q.Set("name", b64Name)
	if token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *connection) readToken() string {
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (c *connection) extractAndSaveToken(data json.RawMessage) {
	var d struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &d); err != nil || d.Token == "" {
		return
	}

	c.logger.Info("Authorization saved — future connections won't need pairing")
	if err := os.WriteFile(c.tokenFile, []byte(d.Token), 0o600); err != nil {
		c.logger.Error("Could not save authorization token", "error", err, "file", c.tokenFile)
	}
}

func (c *connection) dial(ctx context.Context, wsURL string) (*websocket.Conn, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.skipTLSVerify}, //nolint:gosec // Samsung self-signed cert
		},
	}

	dialCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	conn, httpResp, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPClient: client,
	})
	if err != nil {
		return nil, fmt.Errorf("websocket dial: %w", err)
	}
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	conn.SetReadLimit(maxMessageSize)
	return conn, nil
}

func (c *connection) readHandshake(ctx context.Context, conn *websocket.Conn) error {
	readCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, msg, err := conn.Read(readCtx)
	if err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}

	var resp wsResponse
	if err := json.Unmarshal(msg, &resp); err != nil {
		return fmt.Errorf("parse handshake: %w", err)
	}

	switch resp.Event {
	case EventChannelConnect:
		c.extractAndSaveToken(resp.Data)
	case "ms.channel.unauthorized":
		return ErrUnauthorized
	case "ms.channel.timeOut":
		return ErrTimeout
	default:
		return fmt.Errorf("unexpected event %q: %w", resp.Event, ErrConnectionFailure)
	}
	return nil
}

func (c *connection) waitForChannelReady(ctx context.Context, conn *websocket.Conn) error {
	readCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, msg, err := conn.Read(readCtx)
	if err != nil {
		return fmt.Errorf("read channel ready: %w", err)
	}

	var readyResp wsResponse
	if err := json.Unmarshal(msg, &readyResp); err != nil {
		return fmt.Errorf("parse channel ready: %w", err)
	}

	if readyResp.Event != EventChannelReady {
		return fmt.Errorf("expected ms.channel.ready, got %q: %w", readyResp.Event, ErrConnectionFailure)
	}
	return nil
}

func (c *connection) Open(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	token := c.readToken()
	wsURL := c.formatURL(token)
	c.logger.Debug("opening WebSocket", "url", wsURL)

	conn, err := c.dial(ctx, wsURL)
	if err != nil {
		return err
	}

	if err := c.readHandshake(ctx, conn); err != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		return err
	}

	if c.endpoint == endpointArtApp {
		if err := c.waitForChannelReady(ctx, conn); err != nil {
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return err
		}
	}

	c.conn = conn
	c.closed.Store(false)
	c.recvDone = make(chan struct{})
	go c.recvLoop()

	if c.endpoint == endpointArtApp {
		c.logger.Info("Connected to TV for art uploads")
	} else {
		c.logger.Info("Connected to TV")
	}
	return nil
}

func (c *connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	c.closed.Store(true)
	err := c.conn.Close(websocket.StatusNormalClosure, "")
	c.conn = nil

	if c.recvDone != nil {
		select {
		case <-c.recvDone:
		case <-time.After(500 * time.Millisecond):
		}
	}

	c.pendingMu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

	return err
}

func (c *connection) recvLoop() {
	defer close(c.recvDone)

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return
	}

	for {
		_, msg, err := conn.Read(context.Background())
		if err != nil {
			if !c.closed.Load() {
				c.logger.Debug("connection read error", "error", err)
			}
			return
		}

		c.logger.Debug("received message", "payload", truncateWS(string(msg)))

		var resp wsResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}

		if resp.Event == EventD2DServiceMessageEvent || resp.Event == EventD2DServiceMessage {
			c.routeD2DEvent(resp.Data)
		}
	}
}

func (c *connection) routeD2DEvent(dataRaw json.RawMessage) {
	var dataToParse []byte = dataRaw

	var dataStr string
	if err := json.Unmarshal(dataRaw, &dataStr); err == nil {
		dataToParse = []byte(dataStr)
	}

	var inner struct {
		RequestID string `json:"request_id"`
		ID        string `json:"id"`
		Event     string `json:"event"`
	}
	if err := json.Unmarshal(dataToParse, &inner); err != nil {
		return
	}

	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	keys := []string{inner.RequestID, inner.ID, inner.Event}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if ch, ok := c.pending[key]; ok {
			select {
			case ch <- dataToParse:
			default:
			}
			return
		}
	}
}

func artAppRequest(data map[string]any) ([]byte, error) {
	inner, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	outer := map[string]any{
		"method": "ms.channel.emit",
		"params": map[string]any{
			"event": "art_app_request",
			"to":    "host",
			"data":  string(inner),
		},
	}
	return json.Marshal(outer)
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func checkArtError(resp *artResponse) error {
	if resp.ErrorCode != 0 {
		if resp.ErrorCode == 403 || resp.ErrorCode == 507 || resp.ErrorCode == 11001 {
			return fmt.Errorf("%w: code %d", ErrStorageFull, resp.ErrorCode)
		}
		return fmt.Errorf("%w: code %d", ErrArtAPIError, resp.ErrorCode)
	}
	return nil
}

func truncateWS(s string) string {
	if len(s) <= 160 {
		return s
	}
	return s[:160] + "…"
}
