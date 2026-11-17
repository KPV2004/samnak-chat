package main

import (
	"testing"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/eventbus"
)

// busProbe records the bus its module gets in Init.
type busProbe struct {
	fakeModule
	got app.EventBus
}

func (p *busProbe) Init(d *app.Deps) error { p.got = d.Bus; return nil }

func TestModulesGetTheBusFromInfra(t *testing.T) {
	mine := eventbus.New(testLogger())
	probe := &busProbe{fakeModule: fakeModule{name: "probe"}}
	if _, err := newHandler(testLogger(), []string{"probe"}, []app.Module{probe}, infra{Bus: mine}); err != nil {
		t.Fatal(err)
	}
	if probe.got != app.EventBus(mine) {
		t.Fatalf("module got bus %v, want the one in infra", probe.got)
	}
}

func TestNoBusInInfraMeansInMemoryBus(t *testing.T) {
	probe := &busProbe{fakeModule: fakeModule{name: "probe"}}
	if _, err := newHandler(testLogger(), []string{"probe"}, []app.Module{probe}, infra{}); err != nil {
		t.Fatal(err)
	}
	if probe.got == nil {
		t.Fatal("module got no bus")
	}
}
