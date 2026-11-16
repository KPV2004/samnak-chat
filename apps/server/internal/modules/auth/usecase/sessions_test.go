package usecase_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/usecase"
)

const (
	aliceID  = "01920000-0000-7000-8000-000000000001"
	laptopID = "01920000-0000-7000-8000-000000000002"
	phoneID  = "01920000-0000-7000-8000-000000000003"
)

// revokedCall is one DeviceRevoked the service announced.
type revokedCall struct{ userID, deviceID string }

// recorder records the calls the service makes after a commit.
type recorder struct{ calls []revokedCall }

func (r *recorder) DeviceRevoked(_ context.Context, userID, deviceID string) {
	r.calls = append(r.calls, revokedCall{userID: userID, deviceID: deviceID})
}

// tx runs fn unless err is set, like a transaction that fails before the body.
type tx struct{ err error }

func (t tx) Do(ctx context.Context, fn func(context.Context) error) error {
	if t.err != nil {
		return t.err
	}
	return fn(ctx)
}

// repo implements the methods Logout and RevokeDevice use. Any other method
// panics, which keeps a test from silently depending on one.
type repo struct {
	usecase.Repository
	active      []domain.Device
	activeErr   error
	revokeErr   error // RevokeDevice
	revokeCalls int
}

func (r *repo) ActiveDevices(context.Context, string) ([]domain.Device, error) {
	return r.active, r.activeErr
}

func (r *repo) RevokeUserRefreshTokens(context.Context, string, time.Time) error { return nil }

func (r *repo) RevokeUserDevices(context.Context, string, time.Time) error { return nil }

func (r *repo) RevokeDeviceRefreshTokens(context.Context, string, time.Time) error { return nil }

func (r *repo) RevokeDeviceByID(context.Context, string, time.Time) error { return nil }

func (r *repo) RevokeDevice(context.Context, string, string, time.Time) error {
	r.revokeCalls++
	return r.revokeErr
}

func newService(r usecase.Repository, t app.TxManager, ev usecase.Events) *usecase.Service {
	return usecase.NewService(usecase.Deps{
		Repo: r, Tx: t, Events: ev,
		Now: func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
}

func devices(ids ...string) []domain.Device {
	out := make([]domain.Device, len(ids))
	for i, id := range ids {
		out[i] = domain.Device{ID: id, UserID: aliceID}
	}
	return out
}

func TestLogoutOneDevicePublishesDeviceRevoked(t *testing.T) {
	rec := &recorder{}
	svc := newService(&repo{active: devices(laptopID, phoneID)}, tx{}, rec)

	if err := svc.Logout(context.Background(), app.Identity{UserID: aliceID, DeviceID: phoneID}, false); err != nil {
		t.Fatal(err)
	}
	want := []revokedCall{{userID: aliceID, deviceID: phoneID}}
	if len(rec.calls) != 1 || rec.calls[0] != want[0] {
		t.Fatalf("published %v, want %v", rec.calls, want)
	}
}

func TestLogoutAllDevicesPublishesOnceWithoutADevice(t *testing.T) {
	rec := &recorder{}
	svc := newService(&repo{active: devices(laptopID, phoneID)}, tx{}, rec)

	if err := svc.Logout(context.Background(), app.Identity{UserID: aliceID, DeviceID: laptopID}, true); err != nil {
		t.Fatal(err)
	}
	// An empty device ID means every device of the user was revoked.
	if len(rec.calls) != 1 || rec.calls[0] != (revokedCall{userID: aliceID}) {
		t.Fatalf("published %v, want one event for the whole user", rec.calls)
	}
}

func TestLogoutWithNoActiveDevicesPublishesNothing(t *testing.T) {
	rec := &recorder{}
	svc := newService(&repo{}, tx{}, rec)

	if err := svc.Logout(context.Background(), app.Identity{UserID: aliceID, DeviceID: laptopID}, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(context.Background(), app.Identity{UserID: aliceID}, true); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("published %v for a no-op logout", rec.calls)
	}
}

func TestLogoutFailurePublishesNothing(t *testing.T) {
	boom := io.ErrUnexpectedEOF
	for name, r := range map[string]*repo{
		"list devices": {activeErr: boom},
		"commit":       {active: devices(laptopID)},
	} {
		rec := &recorder{}
		svc := newService(r, tx{err: map[string]error{"commit": boom}[name]}, rec)
		if err := svc.Logout(context.Background(), app.Identity{UserID: aliceID, DeviceID: laptopID}, false); err == nil {
			t.Errorf("%s: want an error", name)
		}
		if len(rec.calls) != 0 {
			t.Errorf("%s: published %v after a failure", name, rec.calls)
		}
	}
}

func TestRevokeDevicePublishesDeviceRevoked(t *testing.T) {
	rec := &recorder{}
	r := &repo{}
	svc := newService(r, tx{}, rec)

	if err := svc.RevokeDevice(context.Background(), aliceID, phoneID); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 || rec.calls[0] != (revokedCall{userID: aliceID, deviceID: phoneID}) {
		t.Fatalf("published %v, want one event for the device", rec.calls)
	}
	if r.revokeCalls != 1 {
		t.Fatalf("repository revoke calls = %d, want 1", r.revokeCalls)
	}
}

func TestRevokeDeviceNotFoundPublishesNothing(t *testing.T) {
	rec := &recorder{}
	svc := newService(&repo{revokeErr: app.ErrNotFound}, tx{}, rec)

	err := svc.RevokeDevice(context.Background(), aliceID, phoneID)
	if !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("published %v for an already revoked device", rec.calls)
	}
}

func TestRevokeDeviceFailurePublishesNothing(t *testing.T) {
	rec := &recorder{}
	svc := newService(&repo{revokeErr: io.ErrUnexpectedEOF}, tx{}, rec)

	if err := svc.RevokeDevice(context.Background(), aliceID, phoneID); err == nil {
		t.Fatal("want an error")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("published %v after a failure", rec.calls)
	}
}

func TestRevokeDeviceRejectsAMalformedID(t *testing.T) {
	rec := &recorder{}
	r := &repo{}
	svc := newService(r, tx{}, rec)

	if err := svc.RevokeDevice(context.Background(), aliceID, "not-a-uuid"); err == nil {
		t.Fatal("want an error")
	}
	if r.revokeCalls != 0 || len(rec.calls) != 0 {
		t.Fatalf("malformed id reached the repository (%d calls) or published %v", r.revokeCalls, rec.calls)
	}
}
