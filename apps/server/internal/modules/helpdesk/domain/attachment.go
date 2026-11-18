package domain

import (
	"mime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxUploadBytes is the largest file one ticket may carry (HD-03).
	MaxUploadBytes int64 = 50 << 20
	// MaxAttachments is the most uploaded files one ticket may carry.
	MaxAttachments = 10

	maxFileNameRunes = 255
	maxMimeBytes     = 127
)

// Attachment is a file on a ticket (HD-03). An uploaded file has no ticket
// until the ticket that lists it is created. A file copied from a chat message
// keeps chat's object key and is marked FromMessage; helpdesk only reads it.
type Attachment struct {
	ID          string
	TicketID    string // empty until the ticket is created
	WorkspaceID string
	UploaderID  string
	ObjectKey   string
	FileName    string
	Size        int64
	MimeType    string
	FromMessage bool
	CreatedAt   time.Time
}

// ObjectKey is where an uploaded file lives: helpdesk/{workspace_id}/{attachment_id}.
func ObjectKey(workspaceID, attachmentID string) string {
	return "helpdesk/" + workspaceID + "/" + attachmentID
}

// NormalizeFileName reduces name to its base name and checks it is 1 to 255
// characters without control characters.
func NormalizeFileName(name string) (string, error) {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || utf8.RuneCountInString(name) > maxFileNameRunes {
		return "", invalid("file_name must be 1 to %d characters", maxFileNameRunes)
	}
	if !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return "", invalid("file_name must not contain control characters")
	}
	return name, nil
}

// NormalizeMimeType returns the lower-case "type/subtype" of mimeType and drops
// parameters such as a charset.
func NormalizeMimeType(mimeType string) (string, error) {
	mt, _, err := mime.ParseMediaType(mimeType)
	if err != nil || len(mt) > maxMimeBytes || !strings.Contains(mt, "/") || strings.HasSuffix(mt, "/") {
		return "", invalid("mime_type must look like type/subtype")
	}
	return mt, nil
}

// ValidateUploadSize checks size is between 1 byte and 50 MB.
func ValidateUploadSize(size int64) error {
	if size < 1 || size > MaxUploadBytes {
		return invalid("size must be between 1 byte and %d MB", MaxUploadBytes>>20)
	}
	return nil
}
