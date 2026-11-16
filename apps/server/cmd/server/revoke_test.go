package main

import (
	"context"
	"io"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
)

// fakeRealtime records what the composition root asks it to disconnect.
type fakeRealtime struct {
	devices []string
	users   []string
	// failures let a test check that an error is logged, not returned.
	deviceErr error
	userErr   error
}

func (f *fakeRealtime) SendToUsers(context.Context, []string, app.Envelope) error { return nil }

func (f *fakeRealtime) Online(context.Context, []string) ([]string, error) { return nil, nil }

func (f *fakeRealtime) DisconnectDevice(_ context.Context, deviceID string) error {
	f.devices = append(f.devices, deviceID)
	return f.deviceErr
}

func (f *fakeRealtime) DisconnectUser(_ context.Context, userID string) error {
	f.users = append(f.users, userID)
	return f.userErr
}

func (f *fakeRealtime) Handle(string, app.ClientHandler) {}

// eventWithName is an event that is not auth.DeviceRevoked but has its name.
type eventWithName struct{}

func (eventWithName) EventName() string { return auth.DeviceRevoked{}.EventName() }

func subscribe(t *testing.T, rt app.Realtime) *eventbus.Bus {
	t.Helper()
	bus := eventbus.New(testLogger())
	subscribeDeviceRevoked(testLogger(), bus, rt)
	return bus
}

func TestDeviceRevokedDisconnectsTheDevice(t *testing.T) {
	rt := &fakeRealtime{}
	bus := subscribe(t, rt)

	if err := bus.Publish(context.Background(), auth.DeviceRevoked{UserID: "u1", DeviceID: "d1"}); err != nil {
		t.Fatal(err)
	}
	if len(rt.devices) != 1 || rt.devices[0] != "d1" {
		t.Fatalf("disconnected devices = %v, want [d1]", rt.devices)
	}
	if len(rt.users) != 0 {
		t.Fatalf("disconnected users = %v, want none", rt.users)
	}
}

func TestDeviceRevokedWithoutADeviceDisconnectsTheUser(t *testing.T) {
	rt := &fakeRealtime{}
	bus := subscribe(t, rt)

	if err := bus.Publish(context.Background(), auth.DeviceRevoked{UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	if len(rt.users) != 1 || rt.users[0] != "u1" {
		t.Fatalf("disconnected users = %v, want [u1]", rt.users)
	}
	if len(rt.devices) != 0 {
		t.Fatalf("disconnected devices = %v, want none", rt.devices)
	}
}

func TestOtherEventWithTheSameNameIsIgnored(t *testing.T) {
	rt := &fakeRealtime{}
	bus := subscribe(t, rt)

	if err := bus.Publish(context.Background(), eventWithName{}); err != nil {
		t.Fatal(err)
	}
	if len(rt.devices) != 0 || len(rt.users) != 0 {
		t.Fatalf("disconnected devices %v and users %v for a foreign event", rt.devices, rt.users)
	}
}

// A hub that has no DisconnectUser must not take the server down or fail the
// publisher; the event is only logged.
func TestHubWithoutDisconnectUserOnlyLogs(t *testing.T) {
	bus := subscribe(t, struct{ app.Realtime }{})
	if err := bus.Publish(context.Background(), auth.DeviceRevoked{UserID: "u1"}); err != nil {
		t.Fatalf("publish = %v", err)
	}
}

func TestDisconnectErrorsDoNotFailThePublisher(t *testing.T) {
	rt := &fakeRealtime{deviceErr: io.ErrClosedPipe, userErr: io.ErrClosedPipe}
	bus := subscribe(t, rt)

	if err := bus.Publish(context.Background(), auth.DeviceRevoked{UserID: "u1", DeviceID: "d1"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), auth.DeviceRevoked{UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	if len(rt.devices) != 1 || len(rt.users) != 1 {
		t.Fatalf("disconnects = %v devices, %v users; want both attempted", rt.devices, rt.users)
	}
}

func TestSubscribeWithoutRealtimeDoesNothing(t *testing.T) {
	bus := eventbus.New(testLogger())
	subscribeDeviceRevoked(testLogger(), bus, nil) // must not panic
	if err := bus.Publish(context.Background(), auth.DeviceRevoked{UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
}
