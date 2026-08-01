package bale
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)
const (
	DefaultBaseURL = "https://tapi.bale.ai"
	LongPollTimeout = 25
	MaxUpdatesPerPoll = 100
)
type APIError struct {
	Method      string
	Description string
	Code        int
	RetryAfter  int
}
func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("bale %s failed (%d): %s", e.Method, e.Code, e.Description)
	}
	return fmt.Sprintf("bale %s failed: %s", e.Method, e.Description)
}
func IsTemporary(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == 429 || apiErr.Code >= 500
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	return false
}
func RetryAfterSeconds(err error) float64 {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return float64(apiErr.RetryAfter)
	}
	return 0
}
type apiEnvelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  *apiParameters  `json:"parameters"`
}
type apiParameters struct {
	RetryAfter int `json:"retry_after"`
}
type Client struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}
func NewClient(token, baseURL string, hc *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{Token: token, BaseURL: strings.TrimRight(baseURL, "/"), HTTP: hc}
}
func (c *Client) BotID() int64 {
	id, _ := strconv.ParseInt(strings.SplitN(c.Token, ":", 2)[0], 10, 64)
	return id
}
func (c *Client) endpoint(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.BaseURL, c.Token, method)
}
type formField struct{ Name, Value string }
type filePart struct {
	Field    string
	Filename string
	Reader   io.Reader
}
func (c *Client) doJSON(ctx context.Context, method string, form url.Values) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(method), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, method)
}
func (c *Client) doMultipart(ctx context.Context, method string, fields []formField, files []filePart) (json.RawMessage, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for _, f := range fields {
		if err := w.WriteField(f.Name, f.Value); err != nil {
			return nil, err
		}
	}
	for _, fp := range files {
		part, err := w.CreateFormFile(fp.Field, fp.Filename)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(part, fp.Reader); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(method), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return c.do(req, method)
}
func (c *Client) do(req *http.Request, method string) (json.RawMessage, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("bale: decode %s response: %w", method, err)
	}
	if !env.OK {
		ra := 0
		if env.Parameters != nil {
			ra = env.Parameters.RetryAfter
		}
		return nil, &APIError{Method: method, Description: env.Description, Code: env.ErrorCode, RetryAfter: ra}
	}
	return env.Result, nil
}
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	raw, err := c.doJSON(ctx, "getMe", nil)
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
func (c *Client) GetUpdates(ctx context.Context, offset int64, limit, timeout int) ([]Update, error) {
	form := url.Values{}
	if offset != 0 {
		form.Set("offset", strconv.FormatInt(offset, 10))
	}
	if limit > 0 {
		form.Set("limit", strconv.Itoa(limit))
	}
	if timeout > 0 {
		form.Set("timeout", strconv.Itoa(timeout))
	}
	raw, err := c.doJSON(ctx, "getUpdates", form)
	if err != nil {
		return nil, err
	}
	var updates []Update
	if err := json.Unmarshal(raw, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}
func (c *Client) GetFile(ctx context.Context, fileID string) (*File, error) {
	form := url.Values{}
	form.Set("file_id", fileID)
	raw, err := c.doJSON(ctx, "getFile", form)
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
func (c *Client) DownloadFile(ctx context.Context, f *File, dest string) error {
	link := fmt.Sprintf("%s/file/bot%s/%s", c.BaseURL, c.Token, f.FilePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bale file download failed: %s", resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) (int64, error) {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("text", text)
	raw, err := c.doJSON(ctx, "sendMessage", form)
	if err != nil {
		return 0, err
	}
	var mr MessageResult
	if err := json.Unmarshal(raw, &mr); err != nil {
		return 0, err
	}
	return mr.MessageID, nil
}
func (c *Client) SendMedia(ctx context.Context, method, field string, chatID int64, path, caption string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	fields := []formField{{Name: "chat_id", Value: strconv.FormatInt(chatID, 10)}}
	if caption != "" {
		fields = append(fields, formField{Name: "caption", Value: caption})
	}
	files := []filePart{{Field: field, Filename: filepath.Base(path), Reader: f}}
	raw, err := c.doMultipart(ctx, method, fields, files)
	if err != nil {
		return 0, err
	}
	var mr MessageResult
	if err := json.Unmarshal(raw, &mr); err != nil {
		return 0, err
	}
	return mr.MessageID, nil
}
func (c *Client) SendPhoto(ctx context.Context, chatID int64, path, caption string) (int64, error) {
	return c.SendMedia(ctx, "sendPhoto", "photo", chatID, path, caption)
}
func (c *Client) SendVideo(ctx context.Context, chatID int64, path, caption string) (int64, error) {
	return c.SendMedia(ctx, "sendVideo", "video", chatID, path, caption)
}
func (c *Client) SendDocument(ctx context.Context, chatID int64, path, caption string) (int64, error) {
	return c.SendMedia(ctx, "sendDocument", "document", chatID, path, caption)
}
func (c *Client) SendAudio(ctx context.Context, chatID int64, path, caption string) (int64, error) {
	return c.SendMedia(ctx, "sendAudio", "audio", chatID, path, caption)
}
func (c *Client) SendVoice(ctx context.Context, chatID int64, path, caption string) (int64, error) {
	return c.SendMedia(ctx, "sendVoice", "voice", chatID, path, caption)
}
func (c *Client) SendAnimation(ctx context.Context, chatID int64, path, caption string) (int64, error) {
	return c.SendMedia(ctx, "sendAnimation", "animation", chatID, path, caption)
}
type MediaGroupItem struct {
	Kind    MediaKind
	Path    string
	Caption string
}
func (c *Client) SendMediaGroup(ctx context.Context, chatID int64, items []MediaGroupItem) ([]int64, error) {
	type mediaJSON struct {
		Type    string `json:"type"`
		Media   string `json:"media"`
		Caption string `json:"caption,omitempty"`
	}
	payload := make([]mediaJSON, 0, len(items))
	files := make([]filePart, 0, len(items))
	opened := make([]*os.File, 0, len(items))
	defer func() {
		for _, f := range opened {
			f.Close()
		}
	}()
	for i, it := range items {
		name := fmt.Sprintf("attach_%d", i)
		payload = append(payload, mediaJSON{Type: string(it.Kind), Media: "attach://" + name, Caption: it.Caption})
		f, err := os.Open(it.Path)
		if err != nil {
			return nil, err
		}
		opened = append(opened, f)
		files = append(files, filePart{Field: name, Filename: filepath.Base(it.Path), Reader: f})
	}
	mediaBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	fields := []formField{
		{Name: "chat_id", Value: strconv.FormatInt(chatID, 10)},
		{Name: "media", Value: string(mediaBytes)},
	}
	raw, err := c.doMultipart(ctx, "sendMediaGroup", fields, files)
	if err != nil {
		return nil, err
	}
	var msgs []MessageResult
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.MessageID)
	}
	return ids, nil
}
