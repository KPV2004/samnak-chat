package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace/domain"
)

func TestNormalizeGroupName(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"  Sales  ", "Sales", true},
		{"ฝ่ายขาย", "ฝ่ายขาย", true},
		{strings.Repeat("ก", 100), strings.Repeat("ก", 100), true},
		{"", "", false},
		{"   ", "", false},
		{strings.Repeat("a", 101), "", false},
	}
	for _, c := range cases {
		got, err := domain.NormalizeGroupName(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("NormalizeGroupName(%q) = %q, %v", c.in, got, err)
		}
		if !c.ok && !errors.Is(err, app.ErrInvalid) {
			t.Errorf("NormalizeGroupName(%q) err = %v, want ErrInvalid", c.in, err)
		}
	}
}
