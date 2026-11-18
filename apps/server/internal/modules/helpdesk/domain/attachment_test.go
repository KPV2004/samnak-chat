package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

func TestObjectKeyIsUnderTheModuleAndWorkspace(t *testing.T) {
	if got := domain.ObjectKey(ws, catA); got != "helpdesk/"+ws+"/"+catA {
		t.Errorf("key = %q", got)
	}
}

func TestNormalizeFileNameKeepsTheBaseNameOnly(t *testing.T) {
	cases := map[string]struct {
		in, want string
		wantErr  bool
	}{
		"plain":             {"report.pdf", "report.pdf", false},
		"windows path":      {`C:\Users\me\photo.png`, "photo.png", false},
		"unix path":         {"/tmp/x/notes.txt", "notes.txt", false},
		"blank":             {"   ", "", true},
		"dot":               {".", "", true},
		"double dot":        {"..", "", true},
		"control char":      {"bad\x07name", "", true},
		"too long":          {strings.Repeat("a", 256), "", true},
		"at the length cap": {strings.Repeat("a", 255), strings.Repeat("a", 255), false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := domain.NormalizeFileName(c.in)
			if c.wantErr {
				if !errors.Is(err, app.ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("= %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestNormalizeMimeTypeDropsParameters(t *testing.T) {
	got, err := domain.NormalizeMimeType("Application/PDF; charset=binary")
	if err != nil || got != "application/pdf" {
		t.Fatalf("= %q, %v", got, err)
	}
	for _, bad := range []string{"pdf", "text/", "/pdf", ""} {
		if _, err := domain.NormalizeMimeType(bad); !errors.Is(err, app.ErrInvalid) {
			t.Errorf("%q: err = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestUploadSizeIsBetweenOneByteAndFiftyMegabytes(t *testing.T) {
	for size, ok := range map[int64]bool{0: false, 1: true, domain.MaxUploadBytes: true, domain.MaxUploadBytes + 1: false} {
		if err := domain.ValidateUploadSize(size); (err == nil) != ok {
			t.Errorf("size %d: err = %v, want ok=%v", size, err, ok)
		}
	}
}
