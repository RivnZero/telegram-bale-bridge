package bale

import (
	"context"
	"fmt"
	"os"
	"telegram-bale-bridge/internal/bridge"
)

const (
	MaxPhotoUploadBytes   = 10 * 1024 * 1024
	VoiceAsFileLimitBytes = 1 * 1024 * 1024
)

type Sender struct {
	client *Client
	chatID int64
}

func NewSender(client *Client, chatID int64) *Sender {
	return &Sender{client: client, chatID: chatID}
}

func (s *Sender) Platform() bridge.Platform { return bridge.PlatformBale }
func (s *Sender) ChatID() int64             { return s.chatID }

func (s *Sender) Send(ctx context.Context, msg *bridge.BridgeMessage, path string) (int64, error) {
	switch msg.Kind {
	case bridge.KindText:
		return s.client.SendMessage(ctx, s.chatID, msg.Text)
	case bridge.KindPhoto:
		if tooBig(path, MaxPhotoUploadBytes) {
			return s.client.SendMedia(ctx, "sendDocument", "document", s.chatID, path, msg.Caption)
		}
		return s.client.SendMedia(ctx, "sendPhoto", "photo", s.chatID, path, msg.Caption)
	case bridge.KindVideo:
		return s.client.SendMedia(ctx, "sendVideo", "video", s.chatID, path, msg.Caption)
	case bridge.KindDocument:
		return s.client.SendMedia(ctx, "sendDocument", "document", s.chatID, path, msg.Caption)
	case bridge.KindAudio:
		return s.client.SendMedia(ctx, "sendAudio", "audio", s.chatID, path, msg.Caption)
	case bridge.KindAnimation:
		return s.client.SendMedia(ctx, "sendAnimation", "animation", s.chatID, path, msg.Caption)
	case bridge.KindVoice:
		if tooBig(path, VoiceAsFileLimitBytes) {
			return s.client.SendMedia(ctx, "sendDocument", "document", s.chatID, path, msg.Caption)
		}
		return s.client.SendMedia(ctx, "sendVoice", "voice", s.chatID, path, msg.Caption)
	}
	return 0, fmt.Errorf("unsupported message kind %q", msg.Kind)
}

func (s *Sender) SendAlbum(ctx context.Context, unit *bridge.AlbumUnit, paths []string) ([]int64, error) {
	items := make([]MediaGroupItem, 0, len(unit.Items))
	for i, m := range unit.Items {
		kind := MediaPhoto
		if m.Kind == bridge.KindVideo {
			kind = MediaVideo
		}
		caption := ""
		if i == 0 {
			caption = m.Caption
		}
		items = append(items, MediaGroupItem{Kind: kind, Path: paths[i], Caption: caption})
	}
	return s.client.SendMediaGroup(ctx, s.chatID, items)
}

func tooBig(path string, limit int64) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > limit
}
