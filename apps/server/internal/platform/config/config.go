package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
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
	// WebURL is the base of the links in emails (AUTH-09).
	WebURL string
	// SMTP settings. An empty SMTPHost makes the server log mail instead of
	// sending it. SMTPPort 465 means implicit TLS; other ports use STARTTLS
	// when the server offers it.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	// Modules is app.CoreModules followed by MODULES_ENABLED, without
	// duplicates; app.DefaultModules stand in when MODULES_ENABLED is unset.
	Modules []string
}

// Load reads the configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:     getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		RedisURL:     os.Getenv("REDIS_URL"),
		S3Endpoint:   os.Getenv("S3_ENDPOINT"),
		S3Bucket:     os.Getenv("S3_BUCKET"),
		S3AccessKey:  os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:  os.Getenv("S3_SECRET_KEY"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		WebURL:       getenv("WEB_URL", "http://localhost:5173"),
		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     getenv("SMTP_FROM", "Samnak <no-reply@localhost>"),
		Modules:      ParseModules(os.LookupEnv("MODULES_ENABLED")),
	}
	port, err := strconv.Atoi(getenv("SMTP_PORT", "587"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("config: SMTP_PORT must be a port number, got %q", os.Getenv("SMTP_PORT"))
	}
	cfg.SMTPPort = port
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

// ParseModules returns the core modules followed by the comma-separated
// list, trimmed, lower-cased and de-duplicated. When the variable is not set
// at all, app.DefaultModules are used; set but empty means core modules only.
func ParseModules(list string, set bool) []string {
	mods := slices.Clone(app.CoreModules)
	if !set {
		list = strings.Join(app.DefaultModules, ",")
	}
	for _, name := range strings.Split(list, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && !slices.Contains(mods, name) {
			mods = append(mods, name)
		}
	}
	return mods
}
