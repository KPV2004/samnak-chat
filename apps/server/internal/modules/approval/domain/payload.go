// Package domain holds the approval model and the rules that hold regardless
// of storage (SRS section 12, APR-01..07).
package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Type is the kind of a request. v0.1 has two built-in types (APR-01); custom
// types are APR-12.
type Type string

const (
	TypeLeave   Type = "leave"
	TypeGeneral Type = "general"
)

// ParseType accepts the two built-in types.
func ParseType(s string) (Type, error) {
	switch t := Type(s); t {
	case TypeLeave, TypeGeneral:
		return t, nil
	}
	return "", fmt.Errorf("type must be leave or general: %w", app.ErrInvalid)
}

const (
	maxReasonRunes  = 1000
	maxSubjectRunes = 200
	maxDetailsRunes = 4000
	dateLayout      = time.DateOnly
	defaultCurrency = "THB"
)

// Payload is a validated request payload in the form that is stored: trimmed
// and with defaults filled in.
type Payload struct {
	JSON    json.RawMessage
	Summary string // the leave range or the subject, for cards and notifications
}

// ParsePayload checks raw against the rules of t (APR-01). Unknown fields are
// errors, so a typo does not silently drop what the requester meant.
func ParsePayload(t Type, raw json.RawMessage) (Payload, error) {
	switch t {
	case TypeLeave:
		return parseLeave(raw)
	case TypeGeneral:
		return parseGeneral(raw)
	}
	return Payload{}, fmt.Errorf("type must be leave or general: %w", app.ErrInvalid)
}

// decodeStrict decodes one JSON object into v, refusing unknown fields,
// trailing data and anything that is not an object (null included).
func decodeStrict(raw json.RawMessage, v any) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return fmt.Errorf("payload must be a JSON object: %w", app.ErrInvalid)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("payload: %v: %w", err, app.ErrInvalid)
	}
	if dec.More() {
		return fmt.Errorf("payload must be one JSON object: %w", app.ErrInvalid)
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, app.ErrInvalid)...)
}

// textOK reports whether s is valid text of at most max characters. Newlines
// and tabs are allowed; other control characters are not.
func textOK(s string, max int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' })
}

type leavePayload struct {
	LeaveType string `json:"leave_type"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	HalfDay   string `json:"half_day,omitempty"`
	Reason    string `json:"reason"`
}

func parseLeave(raw json.RawMessage) (Payload, error) {
	var p leavePayload
	if err := decodeStrict(raw, &p); err != nil {
		return Payload{}, err
	}
	switch p.LeaveType {
	case "annual", "sick", "personal", "other":
	default:
		return Payload{}, invalid("leave_type must be annual, sick, personal or other")
	}
	start, err := time.Parse(dateLayout, p.StartDate)
	if err != nil {
		return Payload{}, invalid("start_date must be a date like 2026-12-01")
	}
	end, err := time.Parse(dateLayout, p.EndDate)
	if err != nil {
		return Payload{}, invalid("end_date must be a date like 2026-12-01")
	}
	if end.Before(start) {
		return Payload{}, invalid("end_date must not be before start_date")
	}
	switch p.HalfDay {
	case "":
	case "morning", "afternoon":
		if !start.Equal(end) {
			return Payload{}, invalid("half_day is only for a single day")
		}
	default:
		return Payload{}, invalid("half_day must be morning or afternoon")
	}
	p.Reason = strings.TrimSpace(p.Reason)
	if p.Reason == "" || !textOK(p.Reason, maxReasonRunes) {
		return Payload{}, invalid("reason must be 1 to %d characters", maxReasonRunes)
	}

	summary := p.StartDate
	if !start.Equal(end) {
		summary += " to " + p.EndDate
	}
	if p.HalfDay != "" {
		summary += " (" + p.HalfDay + ")"
	}
	b, err := json.Marshal(p)
	if err != nil {
		return Payload{}, fmt.Errorf("approval: encode payload: %w", err)
	}
	return Payload{JSON: b, Summary: summary}, nil
}

type generalPayload struct {
	Subject  string  `json:"subject"`
	Details  string  `json:"details"`
	Amount   *string `json:"amount,omitempty"`
	Currency *string `json:"currency,omitempty"`
}

var amountPattern = regexp.MustCompile(`^(\d{1,12})(?:\.(\d{1,2}))?$`)

func parseGeneral(raw json.RawMessage) (Payload, error) {
	var p generalPayload
	if err := decodeStrict(raw, &p); err != nil {
		return Payload{}, err
	}
	p.Subject = strings.TrimSpace(p.Subject)
	if p.Subject == "" || !textOK(p.Subject, maxSubjectRunes) || strings.ContainsAny(p.Subject, "\r\n") {
		return Payload{}, invalid("subject must be 1 to %d characters on one line", maxSubjectRunes)
	}
	p.Details = strings.TrimSpace(p.Details)
	if !textOK(p.Details, maxDetailsRunes) {
		return Payload{}, invalid("details must be at most %d characters", maxDetailsRunes)
	}
	switch {
	case p.Amount == nil && p.Currency != nil:
		return Payload{}, invalid("currency needs an amount")
	case p.Amount != nil:
		m := amountPattern.FindStringSubmatch(*p.Amount)
		if m == nil {
			return Payload{}, invalid("amount must be a decimal string of at least 0 with at most 2 decimal places, like \"1200.50\"")
		}
		whole := strings.TrimLeft(m[1], "0")
		if whole == "" {
			whole = "0"
		}
		amount := whole + "." + (m[2] + "00")[:2]
		p.Amount = &amount
		cur := defaultCurrency
		if p.Currency != nil {
			cur = strings.ToUpper(strings.TrimSpace(*p.Currency))
			if len(cur) != 3 || strings.ContainsFunc(cur, func(r rune) bool { return r < 'A' || r > 'Z' }) {
				return Payload{}, invalid("currency must be a 3-letter code like THB")
			}
		}
		p.Currency = &cur
	}
	b, err := json.Marshal(p)
	if err != nil {
		return Payload{}, fmt.Errorf("approval: encode payload: %w", err)
	}
	return Payload{JSON: b, Summary: p.Subject}, nil
}

// NormalizeReason trims a reject reason and checks it is 1 to 1000
// characters (APR-04).
func NormalizeReason(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || !textOK(s, maxReasonRunes) {
		return "", invalid("reason must be 1 to %d characters", maxReasonRunes)
	}
	return s, nil
}
