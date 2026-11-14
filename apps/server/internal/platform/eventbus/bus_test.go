package eventbus

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

type evt string

func (e evt) EventName() string { return string(e) }

func newBus(buf *bytes.Buffer) *Bus {
	return New(slog.New(slog.NewTextHandler(buf, nil)))
}

func TestPublishWithoutSubscribers(t *testing.T) {
	if err := newBus(&bytes.Buffer{}).Publish(context.Background(), evt("a.b")); err != nil {
		t.Fatal(err)
	}
}

func TestHandlersRunInOrderAndOnlyForName(t *testing.T) {
	b := newBus(&bytes.Buffer{})
	var got []int
	b.Subscribe("a.b", func(context.Context, app.Event) error { got = append(got, 1); return nil })
	b.Subscribe("x.y", func(context.Context, app.Event) error { got = append(got, 99); return nil })
	b.Subscribe("a.b", func(context.Context, app.Event) error { got = append(got, 2); return nil })
	b.Subscribe("a.b", func(context.Context, app.Event) error { got = append(got, 3); return nil })
	if err := b.Publish(context.Background(), evt("a.b")); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("order = %v", got)
	}
}

func TestFailingHandlersAreLoggedAndIsolated(t *testing.T) {
	var buf bytes.Buffer
	b := newBus(&buf)
	ran := 0
	b.Subscribe("a.b", func(context.Context, app.Event) error { return errors.New("boom-error") })
	b.Subscribe("a.b", func(context.Context, app.Event) error { panic("boom-panic") })
	b.Subscribe("a.b", func(context.Context, app.Event) error { ran++; return nil })
	if err := b.Publish(context.Background(), evt("a.b")); err != nil {
		t.Fatalf("publish returned %v", err)
	}
	if ran != 1 {
		t.Fatalf("last handler ran %d times", ran)
	}
	for _, s := range []string{"boom-error", "boom-panic", "a.b"} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("log missing %q: %s", s, buf.String())
		}
	}
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	b := newBus(&bytes.Buffer{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			b.Subscribe("a.b", func(context.Context, app.Event) error { return nil })
		}()
		go func() {
			defer wg.Done()
			_ = b.Publish(context.Background(), evt("a.b"))
		}()
	}
	wg.Wait()
}

var _ app.EventBus = (*Bus)(nil)
