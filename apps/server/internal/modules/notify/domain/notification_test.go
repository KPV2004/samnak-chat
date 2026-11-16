package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const (
	ws   = "0193f0f0-0000-7000-8000-000000000001"
	user = "0193f0f0-0000-7000-8000-000000000002"
)

func TestValid(t *testing.T) {
	base := app.Notification{WorkspaceID: ws, UserID: user, Kind: "chat.mention"}
	tests := []struct {
		name string
		n    app.Notification
		want bool
	}{
		{"complete", base, true},
		{"missing user", app.Notification{WorkspaceID: ws, Kind: "chat.mention"}, false},
		{"blank user", app.Notification{WorkspaceID: ws, UserID: "  ", Kind: "chat.mention"}, false},
		{"missing workspace", app.Notification{UserID: user, Kind: "chat.mention"}, false},
		{"missing kind", app.Notification{WorkspaceID: ws, UserID: user}, false},
		{"user is not a uuid", app.Notification{WorkspaceID: ws, UserID: "nope", Kind: "chat.mention"}, false},
		{"workspace is not a uuid", app.Notification{WorkspaceID: "nope", UserID: user, Kind: "chat.mention"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.n); got != tt.want {
				t.Fatalf("Valid(%+v) = %v, want %v", tt.n, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	at := time.Date(2026, 11, 16, 9, 30, 0, 0, time.FixedZone("ICT", 7*60*60))
	id := uuid.Must(uuid.NewV7()).String()
	got := New(app.Notification{
		WorkspaceID: ws, UserID: user, Kind: "chat.dm", Title: "Ada", Body: "hi", Link: "/chat/1",
		Data: map[string]any{"channel_id": "c1"}, Key: "chat.message:m1",
	}, id, at)
	if got.ID != id || got.CreatedAt.Location() != time.UTC || !got.CreatedAt.Equal(at) {
		t.Fatalf("id/time = %q %v, want %q %v", got.ID, got.CreatedAt, id, at)
	}
	if got.Data["channel_id"] != "c1" || got.Key != "chat.message:m1" || got.ReadAt != nil {
		t.Fatalf("unexpected notification: %+v", got)
	}
}

func TestNewFillsEmptyData(t *testing.T) {
	got := New(app.Notification{WorkspaceID: ws, UserID: user, Kind: "chat.mention"}, uuid.NewString(), time.Now())
	if got.Data == nil || len(got.Data) != 0 {
		t.Fatalf("Data = %#v, want an empty map", got.Data)
	}
}
