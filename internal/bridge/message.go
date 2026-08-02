package bridge
import (
	"fmt"
)
type Platform string
const (
	PlatformTelegram Platform = "telegram"
	PlatformBale     Platform = "bale"
)
type MessageKind string
const (
	KindText      MessageKind = "text"
	KindPhoto     MessageKind = "photo"
	KindVideo     MessageKind = "video"
	KindDocument  MessageKind = "document"
	KindAudio     MessageKind = "audio"
	KindVoice     MessageKind = "voice"
	KindAnimation MessageKind = "animation"
)
type BridgeMessage struct {
	SourcePlatform  Platform
	SourceChatID    int64
	SourceMessageID int64
	Kind         MessageKind
	Text         string
	Caption      string
	FileID       string
	FileName     string
	MIMEType     string
	FileSize     int64
	MediaGroupID string
}
func (m *BridgeMessage) DedupeKey() string {
	if m.MediaGroupID != "" {
		return "album:" + m.MediaGroupID
	}
	return fmt.Sprintf("message:%d", m.SourceMessageID)
}
type AlbumKey struct {
	Platform     Platform
	ChatID       int64
	MediaGroupID string
}
type AlbumUnit struct {
	Key   AlbumKey
	Items []*BridgeMessage
}
type SentKey struct {
	Platform  Platform
	ChatID    int64
	MessageID int64
}
