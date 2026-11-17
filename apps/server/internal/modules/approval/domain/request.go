package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Status is where a request stands. Only pending can change, once (APR-04).
type Status string

const (
	StatusPending   Status = "pending"
	StatusApproved  Status = "approved"
	StatusRejected  Status = "rejected"
	StatusCancelled Status = "cancelled"
)

// ParseStatus accepts the four statuses.
func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case StatusPending, StatusApproved, StatusRejected, StatusCancelled:
		return st, nil
	}
	return "", invalid("status must be pending, approved, rejected or cancelled")
}

// Action is a line in a request's history (APR-07).
type Action string

const (
	ActionCreated   Action = "created"
	ActionApproved  Action = "approved"
	ActionRejected  Action = "rejected"
	ActionCancelled Action = "cancelled"
)

// ErrApproverRequired means the request names no approver and the requester
// has no group manager to fall back on (APR-02). It is a 400 with the code
// approver_required.
var ErrApproverRequired = fmt.Errorf("approver_required: choose an approver, you have no group manager: %w", app.ErrInvalid)

// IsApproverRequired reports whether err is ErrApproverRequired.
func IsApproverRequired(err error) bool { return errors.Is(err, ErrApproverRequired) }

// Request is one approval request. Its payload is the validated form of
// ParsePayload.
type Request struct {
	ID          string
	WorkspaceID string
	Type        Type
	RequesterID string
	ApproverID  string
	Status      Status
	Payload     json.RawMessage
	ChannelID   string // the requester-approver DM, once the card is posted
	MessageID   string // the card, once posted
	DecidedAt   *time.Time
	Reason      string // why it was rejected
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Version is what a card carries so a late update never replaces a newer one
// on the client: the row's updated_at in Unix milliseconds.
func (r Request) Version() int64 { return r.UpdatedAt.UnixMilli() }

// CanSee reports whether userID is the requester or the approver. Everyone
// else gets not found (APR-06).
func (r Request) CanSee(userID string) bool {
	return userID != "" && (userID == r.RequesterID || userID == r.ApproverID)
}

// CheckDecide says whether userID may approve or reject now. Only the named
// approver may (403), and only while pending (409) (APR-04).
func (r Request) CheckDecide(userID string) error {
	if userID == "" || userID != r.ApproverID {
		return fmt.Errorf("only the approver can decide this request: %w", app.ErrForbidden)
	}
	return r.checkPending()
}

// CheckCancel says whether userID may cancel now: the requester only, while
// pending (APR-05).
func (r Request) CheckCancel(userID string) error {
	if userID == "" || userID != r.RequesterID {
		return fmt.Errorf("only the requester can cancel this request: %w", app.ErrForbidden)
	}
	return r.checkPending()
}

func (r Request) checkPending() error {
	if r.Status != StatusPending {
		return fmt.Errorf("the request is already %s: %w", r.Status, app.ErrConflict)
	}
	return nil
}

// Step is one stage of approval. v0.1 has exactly one (APR-11 is later).
type Step struct {
	RequestID  string
	StepNo     int
	ApproverID string
	Status     Status
	Reason     string
	DecidedAt  *time.Time
}

// HistoryEntry records who did what, when and why (APR-07).
type HistoryEntry struct {
	ID        string
	RequestID string
	ActorID   string
	Action    Action
	Reason    string
	At        time.Time
}

const (
	// MaxUploadBytes is the largest file one request may carry (APR-09).
	MaxUploadBytes int64 = 50 << 20
	// MaxAttachments is the most files one request may carry.
	MaxAttachments = 10

	maxFileNameRunes = 255
	maxMimeBytes     = 127
)

// Attachment is a file uploaded for a request. It has no request until the
// uploader submits one that lists it.
type Attachment struct {
	ID          string
	RequestID   string // empty until submit
	WorkspaceID string
	UploaderID  string
	ObjectKey   string
	FileName    string
	Size        int64
	MimeType    string
	CreatedAt   time.Time
}

// ObjectKey is where the bytes of an attachment live: approval/{workspace_id}/{attachment_id}.
func ObjectKey(workspaceID, attachmentID string) string {
	return "approval/" + workspaceID + "/" + attachmentID
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

// NormalizeMimeType returns the lower-case "type/subtype" of mimeType and
// drops parameters such as a charset.
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

// Summary is the leave range or the subject, for cards and notifications. It
// is empty when the stored payload cannot be read, which does not happen for
// rows this module wrote.
func (r Request) Summary() string {
	p, err := ParsePayload(r.Type, r.Payload)
	if err != nil {
		return ""
	}
	return p.Summary
}
