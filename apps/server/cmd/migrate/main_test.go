package main

import (
	"embed"
	"reflect"
	"testing"
)

func TestParseModules(t *testing.T) {
	cases := map[string][]string{
		"":                   nil,
		"chat":               {"chat"},
		" chat, helpdesk ,,": {"chat", "helpdesk"},
	}
	for in, want := range cases {
		if got := parseModules(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parseModules(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSelectOwnersRunsOnlyEnabledModulesWithMigrations(t *testing.T) {
	sets := map[string]embed.FS{"chat": {}, "helpdesk": {}}
	got := selectOwners(sets, []string{"helpdesk", "cards"})
	if want := []string{"helpdesk"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
