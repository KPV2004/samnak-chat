package domain

import (
	"fmt"
	"mime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const (
	// MaxUploadBytes is the largest file a message may carry (CHAT-14).
	MaxUploadBytes int64 = 50 << 20
	// MaxAttachments is the most files one message may carry.
	MaxAttachments = 10

	maxFileNameRunes = 255
	maxMimeBytes     = 127
)

// Attachment is a file uploaded to a channel. It has no message until the
// uploader sends one that lists it.
type Attachment struct {
	ID          string
	WorkspaceID string
	ChannelID   string
	UploaderID  string
	MessageID   string // empty until attached
	ObjectKey   string
	FileName    string
	Size        int64
	MimeType    string
	CreatedAt   time.Time
}

// IsImage tells clients to show a preview.
func (a Attachment) IsImage() bool { return strings.HasPrefix(a.MimeType, "image/") }

// ObjectKey is where the bytes of an attachment live: chat/{workspace_id}/{attachment_id}.
func ObjectKey(workspaceID, attachmentID string) string {
	return "chat/" + workspaceID + "/" + attachmentID
}

// NormalizeFileName reduces name to its base name and checks it is 1 to 255
// characters without control characters.
func NormalizeFileName(name string) (string, error) {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || utf8.RuneCountInString(name) > maxFileNameRunes {
		return "", fmt.Errorf("file_name must be 1 to %d characters: %w", maxFileNameRunes, app.ErrInvalid)
	}
	if !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("file_name must not contain control characters: %w", app.ErrInvalid)
	}
	return name, nil
}

// NormalizeMimeType returns the lower-case "type/subtype" of mimeType and
// drops parameters such as a charset.
func NormalizeMimeType(mimeType string) (string, error) {
	mt, _, err := mime.ParseMediaType(mimeType)
	if err != nil || len(mt) > maxMimeBytes || !strings.Contains(mt, "/") || strings.HasSuffix(mt, "/") {
		return "", fmt.Errorf("mime_type must look like type/subtype: %w", app.ErrInvalid)
	}
	return mt, nil
}

// ValidateUploadSize checks size is between 1 byte and 50 MB.
func ValidateUploadSize(size int64) error {
	if size < 1 || size > MaxUploadBytes {
		return fmt.Errorf("size must be between 1 byte and %d MB: %w", MaxUploadBytes>>20, app.ErrInvalid)
	}
	return nil
}
