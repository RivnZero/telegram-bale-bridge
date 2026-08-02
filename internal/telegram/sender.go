package telegram

import (
	"context"
	"errors"
	"fmt"
	"github.com/RivnZero/telegram-bale-bridge/internal/bridge"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"net"
	"os"
	"path/filepath"
)

type Sender struct {
	b      *bot.Bot
	chatID int64
}

func NewSender(b *bot.Bot, chatID int64) *Sender {
	return &Sender{b: b, chatID: chatID}
}
func (s *Sender) Platform() bridge.Platform { return bridge.PlatformTelegram }
func (s *Sender) ChatID() int64             { return s.chatID }
func (s *Sender) Send(ctx context.Context, msg *bridge.BridgeMessage, path string) (int64, error) {
	switch msg.Kind {
	case bridge.KindText:
		m, err := s.b.SendMessage(ctx, &bot.SendMessageParams{ChatID: s.chatID, Text: msg.Text})
		if err != nil {
			return 0, err
		}
		return int64(m.ID), nil
	case bridge.KindPhoto:
		return s.sendFile(ctx, path, func(f *models.InputFileUpload) (*models.Message, error) {
			return s.b.SendPhoto(ctx, &bot.SendPhotoParams{ChatID: s.chatID, Photo: f, Caption: msg.Caption})
		})
	case bridge.KindVideo:
		return s.sendFile(ctx, path, func(f *models.InputFileUpload) (*models.Message, error) {
			return s.b.SendVideo(ctx, &bot.SendVideoParams{ChatID: s.chatID, Video: f, Caption: msg.Caption})
		})
	case bridge.KindDocument:
		return s.sendFile(ctx, path, func(f *models.InputFileUpload) (*models.Message, error) {
			return s.b.SendDocument(ctx, &bot.SendDocumentParams{ChatID: s.chatID, Document: f, Caption: msg.Caption})
		})
	case bridge.KindAudio:
		return s.sendFile(ctx, path, func(f *models.InputFileUpload) (*models.Message, error) {
			return s.b.SendAudio(ctx, &bot.SendAudioParams{ChatID: s.chatID, Audio: f, Caption: msg.Caption})
		})
	case bridge.KindVoice:
		return s.sendFile(ctx, path, func(f *models.InputFileUpload) (*models.Message, error) {
			return s.b.SendVoice(ctx, &bot.SendVoiceParams{ChatID: s.chatID, Voice: f, Caption: msg.Caption})
		})
	case bridge.KindAnimation:
		return s.sendFile(ctx, path, func(f *models.InputFileUpload) (*models.Message, error) {
			return s.b.SendAnimation(ctx, &bot.SendAnimationParams{ChatID: s.chatID, Animation: f, Caption: msg.Caption})
		})
	}
	return 0, fmt.Errorf("unsupported message kind %q", msg.Kind)
}
func (s *Sender) sendFile(ctx context.Context, path string, send func(*models.InputFileUpload) (*models.Message, error)) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	m, err := send(&models.InputFileUpload{Filename: filepath.Base(path), Data: f})
	if err != nil {
		return 0, err
	}
	return int64(m.ID), nil
}
func (s *Sender) SendAlbum(ctx context.Context, unit *bridge.AlbumUnit, paths []string) ([]int64, error) {
	media := make([]models.InputMedia, 0, len(unit.Items))
	opened := make([]*os.File, 0, len(unit.Items))
	defer func() {
		for _, f := range opened {
			f.Close()
		}
	}()
	for i, m := range unit.Items {
		f, err := os.Open(paths[i])
		if err != nil {
			return nil, err
		}
		opened = append(opened, f)
		name := fmt.Sprintf("attach_%d", i)
		caption := ""
		if i == 0 {
			caption = m.Caption
		}
		if m.Kind == bridge.KindVideo {
			media = append(media, &models.InputMediaVideo{Media: "attach://" + name, MediaAttachment: f, Caption: caption})
		} else {
			media = append(media, &models.InputMediaPhoto{Media: "attach://" + name, MediaAttachment: f, Caption: caption})
		}
	}
	msgs, err := s.b.SendMediaGroup(ctx, &bot.SendMediaGroupParams{ChatID: s.chatID, Media: media})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, int64(m.ID))
	}
	return ids, nil
}
func IsTemporary(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if bot.IsTooManyRequestsError(err) {
		return true
	}
	switch {
	case errors.Is(err, bot.ErrorBadRequest),
		errors.Is(err, bot.ErrorForbidden),
		errors.Is(err, bot.ErrorUnauthorized),
		errors.Is(err, bot.ErrorNotFound),
		errors.Is(err, bot.ErrorConflict):
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	return true
}
func RetryAfterSeconds(err error) float64 {
	var tm *bot.TooManyRequestsError
	if errors.As(err, &tm) && tm.RetryAfter > 0 {
		return float64(tm.RetryAfter)
	}
	return 0
}
