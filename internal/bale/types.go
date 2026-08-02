package bale

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}
type Message struct {
	MessageID    int64       `json:"message_id"`
	Date         int64       `json:"date"`
	Chat         Chat        `json:"chat"`
	From         *User       `json:"from"`
	Text         string      `json:"text"`
	Caption      string      `json:"caption"`
	MediaGroupID string      `json:"media_group_id"`
	Animation    *Animation  `json:"animation"`
	Audio        *Audio      `json:"audio"`
	Document     *Document   `json:"document"`
	Photo        []PhotoSize `json:"photo"`
	Video        *Video      `json:"video"`
	Voice        *Voice      `json:"voice"`
}
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
}
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
type PhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}
type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}
type Video struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
	Duration int    `json:"duration"`
}
type Audio struct {
	FileID    string `json:"file_id"`
	FileName  string `json:"file_name"`
	MimeType  string `json:"mime_type"`
	FileSize  int64  `json:"file_size"`
	Duration  int    `json:"duration"`
	Title     string `json:"title"`
	Performer string `json:"performer"`
}
type Voice struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
	Duration int    `json:"duration"`
}
type Animation struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
	Duration int    `json:"duration"`
}
type File struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
	FilePath string `json:"file_path"`
}
type MessageResult struct {
	MessageID int64 `json:"message_id"`
}
type MediaKind string

const (
	MediaPhoto     MediaKind = "photo"
	MediaVideo     MediaKind = "video"
	MediaDocument  MediaKind = "document"
	MediaAudio     MediaKind = "audio"
	MediaAnimation MediaKind = "animation"
)
