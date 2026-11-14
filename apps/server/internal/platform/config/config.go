package config

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Config holds every setting the server reads from the environment.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
	RedisURL    string
	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	JWTSecret   string
	// Modules is app.CoreModules followed by MODULES_ENABLED, without duplicates.
	Modules []string
}

// Load reads the configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Modules:     parseModules(os.Getenv("MODULES_ENABLED")),
	}
	for _, r := range []struct{ key, val string }{
		{"DATABASE_URL", cfg.DatabaseURL},
		{"JWT_SECRET", cfg.JWTSecret},
	} {
		if r.val == "" {
			return Config{}, fmt.Errorf("config: %s is required", r.key)
		}
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseModules returns the core modules followed by the comma-separated
// list, trimmed, lower-cased and de-duplicated.
func parseModules(list string) []string {
	mods := slices.Clone(app.CoreModules)
	for _, name := range strings.Split(list, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && !slices.Contains(mods, name) {
			mods = append(mods, name)
		}
	}
	return mods
}
