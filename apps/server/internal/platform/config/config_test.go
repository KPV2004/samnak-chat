package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func setEnv(t *testing.T, modules string) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("MODULES_ENABLED", modules)
}

func TestLoadModules(t *testing.T) {
	core := []string{"auth", "workspace", "chat"}
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", core},
		{"spaced", " task , approval ", append(append([]string{}, core...), "task", "approval")},
		{"duplicates", "task,task,approval,task", append(append([]string{}, core...), "task", "approval")},
		{"upper-case", "Task,APPROVAL", append(append([]string{}, core...), "task", "approval")},
		{"core repeated", "chat,task", append(append([]string{}, core...), "task")},
		{"empty items", ",task,,", append(append([]string{}, core...), "task")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.in)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.Modules, tt.want) {
				t.Fatalf("Modules = %v, want %v", cfg.Modules, tt.want)
			}
		})
	}
}

func TestLoadModulesUnsetUsesDefaults(t *testing.T) {
	setEnv(t, "")
	os.Unsetenv("MODULES_ENABLED")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"auth", "workspace", "chat", "notify"}
	if !reflect.DeepEqual(cfg.Modules, want) {
		t.Fatalf("Modules = %v, want %v", cfg.Modules, want)
	}
}

func TestLoadRequired(t *testing.T) {
	for _, missing := range []string{"DATABASE_URL", "JWT_SECRET"} {
		t.Run(missing, func(t *testing.T) {
			setEnv(t, "")
			t.Setenv(missing, "")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("err = %v, want one naming %s", err, missing)
			}
		})
	}
}

func TestLoadValuesAndDefaults(t *testing.T) {
	setEnv(t, "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("REDIS_URL", "redis://r")
	t.Setenv("S3_ENDPOINT", "http://s3")
	t.Setenv("S3_BUCKET", "b")
	t.Setenv("S3_ACCESS_KEY", "ak")
	t.Setenv("S3_SECRET_KEY", "sk")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		HTTPAddr: ":8080", DatabaseURL: "postgres://x", RedisURL: "redis://r",
		S3Endpoint: "http://s3", S3Bucket: "b", S3AccessKey: "ak", S3SecretKey: "sk",
		JWTSecret: "secret", Modules: cfg.Modules,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("cfg = %+v, want %+v", cfg, want)
	}

	t.Setenv("HTTP_ADDR", ":9000")
	cfg, _ = Load()
	if cfg.HTTPAddr != ":9000" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
}
