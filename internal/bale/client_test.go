package bale

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testToken = "123:fake-token"

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(testToken, srv.URL, srv.Client())
}

func TestGetMe(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getMe") {
			t.Errorf("path = %q, want getMe", r.URL.Path)
		}
		io.WriteString(w, `{"ok":true,"result":{"id":123,"is_bot":true,"first_name":"BridgeBot"}}`)
	})
	u, err := c.GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if u.ID != 123 || !u.IsBot {
		t.Errorf("user = %+v, want id 123 and is_bot", u)
	}
}

func TestGetUpdates(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.Form.Get("offset") != "10" || r.Form.Get("timeout") != "25" {
			t.Errorf("form = %v, want offset=10 timeout=25", r.Form)
		}
		io.WriteString(w, `{"ok":true,"result":[
			{"update_id":10,"message":{"message_id":1,"chat":{"id":7},"text":"hi"}}
		]}`)
	})
	updates, err := c.GetUpdates(context.Background(), 10, 0, 25)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if len(updates) != 1 || updates[0].Message.Text != "hi" || updates[0].Message.Chat.ID != 7 {
		t.Errorf("updates = %+v, want one message with text hi in chat 7", updates)
	}
}

func TestAPIErrorCarriesRetryAfter(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":5}}`)
	})
	_, err := c.GetMe(context.Background())
	if err == nil {
		t.Fatal("expected API error")
	}
	if !IsTemporary(err) {
		t.Error("IsTemporary = false, want true for 429")
	}
	if got := RetryAfterSeconds(err); got != 5 {
		t.Errorf("RetryAfterSeconds = %v, want 5", got)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 429 {
		t.Errorf("APIError = %+v, want code 429", apiErr)
	}
}

func TestPermanentErrorNotTemporary(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request"}`)
	})
	_, err := c.GetMe(context.Background())
	if err == nil {
		t.Fatal("expected API error")
	}
	if IsTemporary(err) {
		t.Error("IsTemporary = true, want false for 400")
	}
}

func TestSendMediaMultipart(t *testing.T) {
	var gotField, gotCaption, gotFile string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendPhoto") {
			t.Errorf("path = %q, want sendPhoto", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotField = r.FormValue("chat_id")
		gotCaption = r.FormValue("caption")
		f, _, err := r.FormFile("photo")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		gotFile = string(b)
		io.WriteString(w, `{"ok":true,"result":{"message_id":42}}`)
	})
	path := filepath.Join(t.TempDir(), "pic.jpg")
	if err := os.WriteFile(path, []byte("jpeg-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	id, err := c.SendMedia(context.Background(), "sendPhoto", "photo", 7, path, "a caption")
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if id != 42 {
		t.Errorf("message id = %d, want 42", id)
	}
	if gotField != "7" || gotCaption != "a caption" {
		t.Errorf("fields chat_id=%q caption=%q", gotField, gotCaption)
	}
	if gotFile != "jpeg-bytes" {
		t.Errorf("file body = %q, want jpeg-bytes", gotFile)
	}
}

func TestDownloadFile(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getFile"):
			io.WriteString(w, `{"ok":true,"result":{"file_id":"f1","file_path":"photos/1.jpg"}}`)
		case strings.Contains(r.URL.Path, "/file/bot"):
			io.WriteString(w, "file-data")
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	dest := filepath.Join(t.TempDir(), "out.jpg")
	if err := c.DownloadFile(context.Background(), &File{FilePath: "photos/1.jpg"}, dest); err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "file-data" {
		t.Errorf("downloaded = %q, want file-data", b)
	}
}
