package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

func TestParseType(t *testing.T) {
	for _, s := range []string{"leave", "general"} {
		if got, err := domain.ParseType(s); err != nil || string(got) != s {
			t.Errorf("ParseType(%q) = %q, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "Leave", "expense"} {
		if _, err := domain.ParseType(s); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("ParseType(%q) error = %v, want ErrInvalid", s, err)
		}
	}
}

func TestParsePayloadLeave(t *testing.T) {
	ok := []struct{ name, in, summary string }{
		{"range", `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-03","reason":"trip"}`, "2026-12-01 to 2026-12-03"},
		{"one day", `{"leave_type":"sick","start_date":"2026-12-01","end_date":"2026-12-01","reason":" flu "}`, "2026-12-01"},
		{"half day", `{"leave_type":"personal","start_date":"2026-12-01","end_date":"2026-12-01","half_day":"morning","reason":"x"}`, "2026-12-01 (morning)"},
		{"past dates are allowed", `{"leave_type":"other","start_date":"2020-01-01","end_date":"2020-01-02","reason":"x"}`, "2020-01-01 to 2020-01-02"},
	}
	for _, c := range ok {
		t.Run(c.name, func(t *testing.T) {
			p, err := domain.ParsePayload(domain.TypeLeave, json.RawMessage(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if p.Summary != c.summary {
				t.Errorf("summary = %q, want %q", p.Summary, c.summary)
			}
			var got map[string]any
			if err := json.Unmarshal(p.JSON, &got); err != nil {
				t.Fatal(err)
			}
			if c.name == "one day" && got["reason"] != "flu" {
				t.Errorf("reason = %q, want it trimmed", got["reason"])
			}
		})
	}

	bad := []struct{ name, in string }{
		{"not an object", `[]`},
		{"null", `null`},
		{"empty", ``},
		{"unknown leave type", `{"leave_type":"holiday","start_date":"2026-12-01","end_date":"2026-12-01","reason":"x"}`},
		{"missing leave type", `{"start_date":"2026-12-01","end_date":"2026-12-01","reason":"x"}`},
		{"bad start date", `{"leave_type":"annual","start_date":"01/12/2026","end_date":"2026-12-01","reason":"x"}`},
		{"impossible date", `{"leave_type":"annual","start_date":"2026-02-30","end_date":"2026-03-01","reason":"x"}`},
		{"end before start", `{"leave_type":"annual","start_date":"2026-12-02","end_date":"2026-12-01","reason":"x"}`},
		{"half day over a range", `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-02","half_day":"morning","reason":"x"}`},
		{"unknown half day", `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-01","half_day":"evening","reason":"x"}`},
		{"empty reason", `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-01","reason":"  "}`},
		{"reason too long", `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-01","reason":"` + strings.Repeat("a", 1001) + `"}`},
		{"unknown field", `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-01","reason":"x","amount":"5"}`},
		{"wrong field type", `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-01","reason":5}`},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if _, err := domain.ParsePayload(domain.TypeLeave, json.RawMessage(c.in)); !errors.Is(err, app.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestParsePayloadLeaveReasonLengthCountsCharacters(t *testing.T) {
	// 1000 Thai characters are 3000 bytes and must still be accepted.
	in := `{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-01","reason":"` + strings.Repeat("ก", 1000) + `"}`
	if _, err := domain.ParsePayload(domain.TypeLeave, json.RawMessage(in)); err != nil {
		t.Fatal(err)
	}
}

func TestParsePayloadGeneral(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		p, err := domain.ParsePayload(domain.TypeGeneral, json.RawMessage(`{"subject":" Laptop ","details":""}`))
		if err != nil {
			t.Fatal(err)
		}
		if p.Summary != "Laptop" {
			t.Errorf("summary = %q", p.Summary)
		}
		var got map[string]any
		if err := json.Unmarshal(p.JSON, &got); err != nil {
			t.Fatal(err)
		}
		if _, has := got["amount"]; has {
			t.Errorf("amount should be absent: %v", got)
		}
		if got["currency"] != nil {
			t.Errorf("currency without an amount should be absent, got %v", got["currency"])
		}
	})

	amounts := []struct{ in, want string }{
		{"0", "0.00"}, {"12", "12.00"}, {"12.5", "12.50"}, {"1200.05", "1200.05"}, {"007.10", "7.10"},
	}
	for _, c := range amounts {
		t.Run("amount "+c.in, func(t *testing.T) {
			in := `{"subject":"s","details":"d","amount":"` + c.in + `"}`
			p, err := domain.ParsePayload(domain.TypeGeneral, json.RawMessage(in))
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(p.JSON, &got); err != nil {
				t.Fatal(err)
			}
			if got["amount"] != c.want {
				t.Errorf("amount = %v, want %s", got["amount"], c.want)
			}
			if got["currency"] != "THB" {
				t.Errorf("currency = %v, want the THB default", got["currency"])
			}
		})
	}

	t.Run("currency is kept and upper-cased", func(t *testing.T) {
		p, err := domain.ParsePayload(domain.TypeGeneral, json.RawMessage(`{"subject":"s","amount":"5","currency":"usd"}`))
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		_ = json.Unmarshal(p.JSON, &got)
		if got["currency"] != "USD" {
			t.Errorf("currency = %v", got["currency"])
		}
	})

	bad := []struct{ name, in string }{
		{"no subject", `{"details":"d"}`},
		{"blank subject", `{"subject":"  "}`},
		{"subject too long", `{"subject":"` + strings.Repeat("a", 201) + `"}`},
		{"details too long", `{"subject":"s","details":"` + strings.Repeat("a", 4001) + `"}`},
		{"negative amount", `{"subject":"s","amount":"-1"}`},
		{"three decimals", `{"subject":"s","amount":"1.234"}`},
		{"not a number", `{"subject":"s","amount":"ten"}`},
		{"amount as number", `{"subject":"s","amount":10}`},
		{"empty amount", `{"subject":"s","amount":""}`},
		{"huge amount", `{"subject":"s","amount":"` + strings.Repeat("9", 20) + `"}`},
		{"bad currency", `{"subject":"s","amount":"1","currency":"BAHT"}`},
		{"currency without amount", `{"subject":"s","currency":"THB"}`},
		{"unknown field", `{"subject":"s","start_date":"2026-12-01"}`},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if _, err := domain.ParsePayload(domain.TypeGeneral, json.RawMessage(c.in)); !errors.Is(err, app.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestParsePayloadUnknownType(t *testing.T) {
	if _, err := domain.ParsePayload("expense", json.RawMessage(`{}`)); !errors.Is(err, app.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}
