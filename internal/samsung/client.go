package samsung

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options configures a Samsung Frame TV client.
type Options struct {
	Host            string
	ClientName      string
	TokenDir        string
	Matte           string
	Timeout         time.Duration
	SkipTLSVerify   bool
	Logger          *slog.Logger
}

// Client talks to a Samsung Frame TV over the art-app WebSocket channel.
type Client struct {
	host string
	opts Options
	log  *slog.Logger

	artConn *connection
}

// NewClient creates a client. Call Connect before other operations.
func NewClient(opts Options) *Client {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.ClientName == "" {
		opts.ClientName = "frame-it"
	}
	if opts.Matte == "" {
		opts.Matte = "none"
	}
	if opts.TokenDir == "" {
		opts.TokenDir = ".frame-it"
	}

	return &Client{
		host: opts.Host,
		opts: opts,
		log:  log, // host is included in messages when relevant
	}
}

func (c *Client) tokenFilePath() string {
	safeIP := strings.ReplaceAll(c.host, ".", "_")
	return filepath.Join(c.opts.TokenDir, fmt.Sprintf("tv_%s.txt", safeIP))
}

// Pair opens the remote-control channel to obtain an auth token.
// The TV shows an on-screen prompt — accept it with the remote.
func (c *Client) Pair(ctx context.Context) error {
	tokenFile := c.tokenFilePath()
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		return fmt.Errorf("create token dir: %w", err)
	}

	conn := newConnection(connConfig{
		host:          c.host,
		port:          portArtWSS,
		endpoint:      endpointRemoteControl,
		name:          c.opts.ClientName,
		tokenFile:     tokenFile,
		timeout:       c.opts.Timeout,
		skipTLSVerify: c.opts.SkipTLSVerify,
		logger:        c.log,
	})

	c.log.Info("Look at the TV and press Allow on the authorization prompt")

	if err := conn.Open(ctx); err != nil {
		return err
	}
	return conn.Close()
}

// Connect opens the art-app WebSocket channel.
func (c *Client) Connect(ctx context.Context) error {
	tokenFile := c.tokenFilePath()

	if _, err := os.Stat(tokenFile); os.IsNotExist(err) {
		c.log.Info("No saved authorization — accept the Allow prompt on the TV if one appears")
	}

	c.log.Info("Connecting to TV", "host", c.host)

	c.artConn = newConnection(connConfig{
		host:          c.host,
		port:          portArtWSS,
		endpoint:      endpointArtApp,
		name:          c.opts.ClientName,
		tokenFile:     tokenFile,
		timeout:       c.opts.Timeout,
		skipTLSVerify: c.opts.SkipTLSVerify,
		logger:        c.log,
	})

	return c.artConn.Open(ctx)
}

// Close shuts down the art connection.
func (c *Client) Close() error {
	if c.artConn != nil {
		return c.artConn.Close()
	}
	return nil
}

// APIVersion returns the TV art API version (e.g. "0.97" on 2018–2019 models).
func (c *Client) APIVersion(ctx context.Context) (string, error) {
	id := newRequestID()
	req := map[string]any{
		"request":    "api_version",
		"id":         id,
		"request_id": id,
	}

	resp, _, err := c.sendArtRequest(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Value, nil
}

// ListUploaded returns user-uploaded artwork (category MY-C0002 = My Photos).
func (c *Client) ListUploaded(ctx context.Context) ([]ArtContent, error) {
	id := newRequestID()
	req := map[string]any{
		"request":     "get_content_list",
		"id":          id,
		"request_id":  id,
		"category_id": "MY-C0002",
	}

	resp, _, err := c.sendArtRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	contentListStr := resp.ContentList()
	if contentListStr == "" {
		return nil, nil
	}

	var items []ArtContent
	if err := json.Unmarshal([]byte(contentListStr), &items); err != nil {
		return nil, fmt.Errorf("parse content_list: %w", err)
	}

	filtered := make([]ArtContent, 0, len(items))
	for _, item := range items {
		if item.CategoryID == "MY-C0002" {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

// DeleteImages removes artwork by content IDs.
func (c *Client) DeleteImages(ctx context.Context, ids []string) error {
	id := newRequestID()

	contentIDList := make([]map[string]string, len(ids))
	for i, cid := range ids {
		contentIDList[i] = map[string]string{"content_id": cid}
	}

	req := map[string]any{
		"request":         "delete_image_list",
		"id":              id,
		"request_id":      id,
		"content_id_list": contentIDList,
	}

	_, _, err := c.sendArtRequest(ctx, req)
	return err
}

// SelectImage displays the artwork with the given content ID.
func (c *Client) SelectImage(ctx context.Context, contentID string) error {
	reqID := newRequestID()
	req := map[string]any{
		"request":    "select_image",
		"id":         reqID,
		"request_id": reqID,
		"content_id": contentID,
		"show":       true,
	}

	_, _, err := c.sendArtRequest(ctx, req)
	return err
}

// Upload sends an image file to the TV and returns the assigned content ID.
func (c *Client) Upload(ctx context.Context, filePath string) (string, error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", filePath, err)
	}

	fileType := fileTypeFromPath(filePath)
	matte := c.opts.Matte

	waitForAdded := c.registerImageAddedListener()

	apiVersion, apiErr := c.APIVersion(ctx)
	if apiErr == nil && apiVersion == "0.97" {
		c.log.Debug("Using legacy upload method for this TV (API 0.97)")
		return c.uploadBinary(ctx, filePath, fileType, matte, stat.Size(), waitForAdded)
	}

	return c.uploadD2D(ctx, filePath, fileType, matte, stat.Size(), waitForAdded)
}

func (c *Client) uploadD2D(
	ctx context.Context,
	filePath, fileType, matte string,
	fileSize int64,
	waitForAdded func(ctx context.Context, timeout time.Duration) (string, error),
) (string, error) {
	id := newRequestID()
	resp, _, err := c.sendArtRequest(ctx, buildSendImageRequest(id, fileType, matte, fileSize))
	if err != nil {
		return "", err
	}

	connInfoStr := resp.ConnInfo()
	if connInfoStr == "" {
		return "", fmt.Errorf("send_image: no conn_info in response")
	}

	var ci connInfo
	if err := json.Unmarshal([]byte(connInfoStr), &ci); err != nil {
		return "", fmt.Errorf("parse conn_info: %w", err)
	}

	if err := uploadImageD2D(ctx, d2dUpload{
		info:          ci,
		filePath:      filePath,
		fileType:      fileType,
		timeout:       c.opts.Timeout,
		skipTLSVerify: c.opts.SkipTLSVerify,
	}); err != nil {
		return "", fmt.Errorf("d2d transfer: %w", err)
	}

	contentID, err := waitForAdded(ctx, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("wait for confirmation: %w", err)
	}
	return contentID, nil
}

func (c *Client) uploadBinary(
	ctx context.Context,
	filePath, fileType, matte string,
	_ int64,
	waitForAdded func(ctx context.Context, timeout time.Duration) (string, error),
) (string, error) {
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}

	uploadID := newRequestID()
	ftHdr := fileTypeHeader(fileType)

	inner := map[string]any{
		"request":   "send_image",
		"file_type": ftHdr,
		"matte_id":  matte,
		"id":        uploadID,
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return "", err
	}

	outer := map[string]any{
		"method": "ms.channel.emit",
		"params": map[string]any{
			"data":  string(innerJSON),
			"to":    "host",
			"event": "art_app_request",
		},
	}
	header, err := json.Marshal(outer)
	if err != nil {
		return "", err
	}
	if len(header) > 0xFFFF {
		return "", fmt.Errorf("upload header too large")
	}

	payload := make([]byte, 2+len(header)+len(data))
	binary.BigEndian.PutUint16(payload[0:2], uint16(len(header))) //nolint:gosec // bounded above
	copy(payload[2:], header)
	copy(payload[2+len(header):], data)

	if err := c.artConn.SendBinary(ctx, payload); err != nil {
		return "", err
	}

	contentID, err := waitForAdded(ctx, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("wait for confirmation: %w", err)
	}
	return contentID, nil
}

func (c *Client) registerImageAddedListener() func(ctx context.Context, timeout time.Duration) (string, error) {
	ch := make(chan json.RawMessage, 1)

	c.artConn.pendingMu.Lock()
	c.artConn.pending["image_added"] = ch
	c.artConn.pendingMu.Unlock()

	return func(ctx context.Context, timeout time.Duration) (string, error) {
		defer func() {
			if c.artConn != nil {
				c.artConn.pendingMu.Lock()
				delete(c.artConn.pending, "image_added")
				c.artConn.pendingMu.Unlock()
			}
		}()

		select {
		case data, ok := <-ch:
			if !ok {
				return "", ErrNotConnected
			}
			var resp artResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return "", fmt.Errorf("parse image_added: %w", err)
			}
			if err := checkArtError(&resp); err != nil {
				return "", err
			}
			return resp.ContentID, nil
		case <-time.After(timeout):
			return "", fmt.Errorf("%w: waiting for image_added", ErrTimeout)
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func buildSendImageRequest(id, fileType, matte string, fileSize int64) map[string]any {
	const connectionIDModulus = 4 * 1024 * 1024 * 1024

	return map[string]any{
		"request":    "send_image",
		"file_type":  fileType,
		"id":         id,
		"request_id": id,
		"conn_info": map[string]any{
			"d2d_mode":        "socket",
			"connection_id": time.Now().UnixNano() % connectionIDModulus,
			"id":              id,
		},
		"image_date":        time.Now().Format("2006:01:02 15:04:05"),
		"matte_id":          matte,
		"portrait_matte_id": matte,
		"file_size":         fileSize,
	}
}

func (c *Client) sendArtRequest(ctx context.Context, req map[string]any) (*artResponse, json.RawMessage, error) {
	name := fmt.Sprint(req["request"])
	reqID := fmt.Sprint(req["request_id"])

	payload, err := artAppRequest(req)
	if err != nil {
		return nil, nil, fmt.Errorf("build %s request: %w", name, err)
	}

	raw, err := c.artConn.SendAndWait(ctx, payload, reqID, c.opts.Timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}

	var resp artResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("parse %s response: %w", name, err)
	}

	if err := checkArtError(&resp); err != nil {
		return nil, nil, fmt.Errorf("%s error: %w", name, err)
	}

	return &resp, raw, nil
}

func fileTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "JPEG"
	case ".png":
		return "PNG"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

func fileTypeHeader(fileType string) string {
	ft := strings.ToLower(fileType)
	if ft == "jpg" || ft == "jpeg" {
		return "JPEG"
	}
	return strings.ToUpper(fileType)
}
