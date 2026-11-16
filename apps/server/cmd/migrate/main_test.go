package main

import (
	"embed"
	"os"
	"reflect"
	"testing"
)

func TestLoadSettingsMigratesDefaultModulesWhenUnset(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	os.Unsetenv("MODULES_ENABLED")
	got, err := loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"auth", "workspace", "chat", "notify"}; !reflect.DeepEqual(got.Modules, want) {
		t.Fatalf("Modules = %v, want %v", got.Modules, want)
	}
}

func TestSelectOwnersRunsOnlyEnabledModulesWithMigrations(t *testing.T) {
	sets := map[string]embed.FS{"chat": {}, "helpdesk": {}}
	got := selectOwners(sets, []string{"helpdesk", "cards"})
	if want := []string{"helpdesk"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
