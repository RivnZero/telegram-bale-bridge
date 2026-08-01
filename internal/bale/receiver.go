package bale
import (
	"context"
	"telegram-bale-bridge/internal/bridge"
)
type Receiver struct {
	client  *Client
	chatID  int64
	botID   int64
	tracker *bridge.SentTracker
	submit  func(context.Context, *bridge.BridgeMessage)
}
func NewReceiver(client *Client, chatID, botID int64, tracker *bridge.SentTracker, submit func(context.Context, *bridge.BridgeMessage)) *Receiver {
	return &Receiver{client: client, chatID: chatID, botID: botID, tracker: tracker, submit: submit}
}
func (r *Receiver) Handle(ctx context.Context, m *Message) {
	if m.Chat.ID != r.chatID {
		return
	}
	if m.From != nil && (m.From.IsBot || m.From.ID == r.botID) {
		return
	}
	if r.tracker.Seen(bridge.PlatformBale, m.Chat.ID, m.MessageID) {
		return
	}
	if bm := Normalize(m); bm != nil {
		r.submit(ctx, bm)
	}
}
func (r *Receiver) Download(ctx context.Context, fileID, dest string) error {
	f, err := r.client.GetFile(ctx, fileID)
	if err != nil {
		return err
	}
	return r.client.DownloadFile(ctx, f, dest)
}
func Normalize(m *Message) *bridge.BridgeMessage {
	bm := &bridge.BridgeMessage{
		SourcePlatform:  bridge.PlatformBale,
		SourceChatID:    m.Chat.ID,
		SourceMessageID: m.MessageID,
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
		bm.FileID, bm.FileSize = p.FileID, p.FileSize
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
