package config

import (
	"fmt"
	"net/netip"
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
	// S3PublicEndpoint is the storage URL browsers use for presigned URLs
	// when it differs from S3Endpoint (behind a reverse proxy). Optional.
	S3PublicEndpoint string
	S3Bucket         string
	S3AccessKey      string
	S3SecretKey      string
	JWTSecret        string
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
	// RateLimit is the API-wide limit on /api/v1 (NFR-07).
	RateLimit RateLimit
}

// RateLimit sets the API-wide limit on /api/v1. Both limits count calls per
// minute. Login keeps its own, stricter limit (AUTH-06).
type RateLimit struct {
	// Enabled is RATE_LIMIT_ENABLED. Off means no API-wide limit.
	Enabled bool
	// PerUser is RATE_LIMIT_PER_MINUTE: calls a signed-in user may make.
	PerUser int
	// PerIP is RATE_LIMIT_IP_PER_MINUTE: calls a signed-out client may make.
	PerIP int
	// TrustedProxies is TRUSTED_PROXIES: the addresses whose X-Forwarded-For
	// is believed when a client is keyed by IP, such as the web service of
	// compose.prod. Empty trusts none.
	TrustedProxies []netip.Prefix
}

// Load reads the configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:         getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		S3Endpoint:       os.Getenv("S3_ENDPOINT"),
		S3PublicEndpoint: os.Getenv("S3_PUBLIC_ENDPOINT"),
		S3Bucket:         os.Getenv("S3_BUCKET"),
		S3AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:      os.Getenv("S3_SECRET_KEY"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		WebURL:           getenv("WEB_URL", "http://localhost:5173"),
		SMTPHost:         os.Getenv("SMTP_HOST"),
		SMTPUsername:     os.Getenv("SMTP_USERNAME"),
		SMTPPassword:     os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:         getenv("SMTP_FROM", "Samnak <no-reply@localhost>"),
		Modules:          ParseModules(os.LookupEnv("MODULES_ENABLED")),
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
	rl, err := loadRateLimit()
	if err != nil {
		return Config{}, err
	}
	cfg.RateLimit = rl
	return cfg, nil
}

func loadRateLimit() (RateLimit, error) {
	enabled, err := strconv.ParseBool(getenv("RATE_LIMIT_ENABLED", "true"))
	if err != nil {
		return RateLimit{}, fmt.Errorf("config: RATE_LIMIT_ENABLED must be true or false, got %q", os.Getenv("RATE_LIMIT_ENABLED"))
	}
	perUser, err := positiveInt("RATE_LIMIT_PER_MINUTE", "600")
	if err != nil {
		return RateLimit{}, err
	}
	perIP, err := positiveInt("RATE_LIMIT_IP_PER_MINUTE", "120")
	if err != nil {
		return RateLimit{}, err
	}
	proxies, err := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return RateLimit{}, err
	}
	return RateLimit{Enabled: enabled, PerUser: perUser, PerIP: perIP, TrustedProxies: proxies}, nil
}

func positiveInt(key, def string) (int, error) {
	n, err := strconv.Atoi(getenv(key, def))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("config: %s must be a whole number of at least 1, got %q", key, os.Getenv(key))
	}
	return n, nil
}

// parseTrustedProxies reads comma-separated IP addresses or CIDR prefixes. A
// bare address is a prefix of one address.
func parseTrustedProxies(list string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			p, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, fmt.Errorf("config: TRUSTED_PROXIES: %q is not an address or prefix: %w", item, err)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("config: TRUSTED_PROXIES: %q is not an address or prefix: %w", item, err)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
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
