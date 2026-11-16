package auth_test

import (
	"context"
	"sync"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
)

// revokedEvents collects the auth.device_revoked events an env publishes.
type revokedEvents struct {
	mu  sync.Mutex
	got []auth.DeviceRevoked
}

func (c *revokedEvents) subscribe(e *env) {
	e.bus.Subscribe(auth.DeviceRevoked{}.EventName(), func(_ context.Context, evt app.Event) error {
		d, ok := evt.(auth.DeviceRevoked)
		if !ok {
			return nil
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.got = append(c.got, d)
		return nil
	})
}

func (c *revokedEvents) all() []auth.DeviceRevoked {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]auth.DeviceRevoked{}, c.got...)
}

func (c *revokedEvents) want(t *testing.T, want ...auth.DeviceRevoked) {
	t.Helper()
	got := c.all()
	if len(got) != len(want) {
		t.Fatalf("published %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("published %v, want %v", got, want)
		}
	}
}

func TestDeviceRevokedEventName(t *testing.T) {
	if got := (auth.DeviceRevoked{}).EventName(); got != "auth.device_revoked" {
		t.Fatalf("event name = %q", got)
	}
}

func TestLogoutPublishesDeviceRevoked(t *testing.T) {
	e := newEnv(t)
	events := &revokedEvents{}
	events.subscribe(e)
	a := e.register("Alice")
	laptop := e.login(a, "Laptop")
	userID := e.do("GET", "/auth/me", req{Token: laptop.Access}).String("id")

	if r := e.do("POST", "/auth/logout", req{Token: laptop.Access, Body: map[string]any{"all_devices": false}}); r.Status != 204 {
		t.Fatalf("logout: %d %s", r.Status, r.Body)
	}
	events.want(t, auth.DeviceRevoked{UserID: userID, DeviceID: laptop.DeviceID})

	// A replay cannot authenticate, and publishes nothing more.
	if r := e.do("POST", "/auth/logout", req{Token: laptop.Access, Body: map[string]any{}}); r.Status != 401 {
		t.Fatalf("second logout: %d %s", r.Status, r.Body)
	}
	events.want(t, auth.DeviceRevoked{UserID: userID, DeviceID: laptop.DeviceID})
}

func TestLogoutAllDevicesPublishesOneEventForTheUser(t *testing.T) {
	e := newEnv(t)
	events := &revokedEvents{}
	events.subscribe(e)
	a := e.register("Alice")
	laptop := e.login(a, "Laptop")
	e.login(a, "Phone")
	userID := e.do("GET", "/auth/me", req{Token: laptop.Access}).String("id")

	if r := e.do("POST", "/auth/logout", req{Token: laptop.Access, Body: map[string]any{"all_devices": true}}); r.Status != 204 {
		t.Fatalf("logout: %d %s", r.Status, r.Body)
	}
	// No device ID: every device of the user went.
	events.want(t, auth.DeviceRevoked{UserID: userID})
}

func TestRevokeDevicePublishesDeviceRevoked(t *testing.T) {
	e := newEnv(t)
	events := &revokedEvents{}
	events.subscribe(e)
	a := e.register("Alice")
	laptop := e.login(a, "Laptop")
	phone := e.login(a, "Phone")
	userID := e.do("GET", "/auth/me", req{Token: laptop.Access}).String("id")

	if r := e.do("DELETE", "/auth/devices/"+phone.DeviceID, req{Token: laptop.Access}); r.Status != 204 {
		t.Fatalf("revoke: %d %s", r.Status, r.Body)
	}
	events.want(t, auth.DeviceRevoked{UserID: userID, DeviceID: phone.DeviceID})

	// Already revoked: 404 and no second event.
	if r := e.do("DELETE", "/auth/devices/"+phone.DeviceID, req{Token: laptop.Access}); r.Status != 404 {
		t.Fatalf("second revoke: %d %s", r.Status, r.Body)
	}
	events.want(t, auth.DeviceRevoked{UserID: userID, DeviceID: phone.DeviceID})
}

func TestRevokeAnotherUsersDevicePublishesNothing(t *testing.T) {
	e := newEnv(t)
	events := &revokedEvents{}
	events.subscribe(e)
	alice := e.login(e.register("Alice"), "Laptop")
	bob := e.login(e.register("Bob"), "Laptop")

	if r := e.do("DELETE", "/auth/devices/"+bob.DeviceID, req{Token: alice.Access}); r.Status != 404 {
		t.Fatalf("revoke: %d %s", r.Status, r.Body)
	}
	events.want(t)
}
