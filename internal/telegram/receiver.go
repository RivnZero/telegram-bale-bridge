package telegram

import (
	"context"
	"fmt"
	"github.com/RivnZero/telegram-bale-bridge/internal/bridge"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"io"
	"net/http"
	"os"
	"time"
)

type Receiver struct {
	b       *bot.Bot
	chatID  int64
	tracker *bridge.SentTracker
	submit  func(context.Context, *bridge.BridgeMessage)
}

func NewReceiver(b *bot.Bot, chatID int64, tracker *bridge.SentTracker, submit func(context.Context, *bridge.BridgeMessage)) *Receiver {
	return &Receiver{b: b, chatID: chatID, tracker: tracker, submit: submit}
}
func (r *Receiver) Handle(ctx context.Context, update *models.Update) {
	m := update.Message
	if m == nil {
		return
	}
	if r.chatID != 0 && m.Chat.ID != r.chatID {
		return
	}
	if m.From != nil && (m.From.IsBot || m.From.ID == r.b.ID()) {
		return
	}
	if r.tracker.Seen(bridge.PlatformTelegram, m.Chat.ID, int64(m.ID)) {
		return
	}
	if bm := Normalize(m); bm != nil {
		r.submit(ctx, bm)
	}
}
func (r *Receiver) Download(ctx context.Context, fileID, dest string) error {
	f, err := r.b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return err
	}
	link := r.b.FileDownloadLink(f)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram file download failed: %s", resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
func Normalize(m *models.Message) *bridge.BridgeMessage {
	bm := &bridge.BridgeMessage{
		SourcePlatform:  bridge.PlatformTelegram,
		SourceChatID:    m.Chat.ID,
		SourceMessageID: int64(m.ID),
		Caption:         m.Caption,
		MediaGroupID:    m.MediaGroupID,
	}
	switch {
	case m.Text != "":
		bm.Kind = bridge.KindText
		bm.Text = m.Text
	case m.Animation != nil:
		bm.Kind = bridge.KindAnimation
		bm.FileID, bm.FileName, bm.MIMEType, bm.FileSize = m.Animation.FileID, m.Animation.FileName, m.Animation.MimeType, m.Animation.FileSize
	case m.Audio != nil:
		bm.Kind = bridge.KindAudio
		bm.FileID, bm.FileName, bm.MIMEType, bm.FileSize = m.Audio.FileID, m.Audio.FileName, m.Audio.MimeType, m.Audio.FileSize
	case m.Document != nil:
		bm.Kind = bridge.KindDocument
		bm.FileID, bm.FileName, bm.MIMEType, bm.FileSize = m.Document.FileID, m.Document.FileName, m.Document.MimeType, m.Document.FileSize
	case len(m.Photo) > 0:
		p := m.Photo[len(m.Photo)-1]
		bm.Kind = bridge.KindPhoto
		bm.FileID, bm.FileSize = p.FileID, int64(p.FileSize)
	case m.Video != nil:
		bm.Kind = bridge.KindVideo
		bm.FileID, bm.FileName, bm.MIMEType, bm.FileSize = m.Video.FileID, m.Video.FileName, m.Video.MimeType, m.Video.FileSize
	case m.Voice != nil:
		bm.Kind = bridge.KindVoice
		bm.FileID, bm.MIMEType, bm.FileSize = m.Voice.FileID, m.Voice.MimeType, m.Voice.FileSize
	default:
		return nil // sticker, contact, location, service message, ...
	}
	return bm
}
