package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

func TestOpenRedisRejectsMissingURL(t *testing.T) {
	_, err := openRedis(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("err = %v, want one naming REDIS_URL", err)
	}
}

func TestOpenRedisFailsClearlyWhenUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := openRedis(ctx, "redis://127.0.0.1:1/0")
	if err == nil || !strings.Contains(err.Error(), "Redis") || !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("err = %v, want one naming Redis and REDIS_URL", err)
	}
}

// Startup must stop on an unreachable Redis before it serves anything.
func TestRunFailsWhenRedisIsUnreachable(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x:x@127.0.0.1:1/x")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("REDIS_URL", "redis://127.0.0.1:1/0")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	err := run(context.Background(), testLogger(), nil)
	if err == nil || !strings.Contains(err.Error(), "Redis") {
		t.Fatalf("err = %v, want one naming Redis", err)
	}
}

func TestReadyzReportsRedis(t *testing.T) {
	client := startRedis(t)
	h, err := newHandler(testLogger(), nil, nil, infra{}.withRedis(client))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 200 {
		t.Fatalf("readyz = %d %s, want 200", rec.Code, rec.Body)
	}

	_ = client.Close()
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), "redis") {
		t.Fatalf("readyz after close = %d %s, want 503 naming redis", rec.Code, rec.Body)
	}
}

// auth must pick the Redis limiter when Deps.Platform["redis"] is set: the
// failures of a bad login show up as a key in Redis.
func TestAuthCountsFailedLoginsInRedis(t *testing.T) {
	client := startRedis(t)
	pool := startPostgres(t)
	h, err := newHandler(testLogger(), []string{"auth"}, []app.Module{auth.New()}, infra{
		Tx:       database.NewTxManager(pool),
		Platform: map[string]any{"db": pool, "jwt_secret": "test-secret-test-secret-test-secret"},
	}.withRedis(client))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"email":"nobody@example.com","password":"wrong-password","device_name":"t","platform":"web"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("login = %d %s", rec.Code, rec.Body)
	}
	keys := client.Keys(context.Background(), "*").Val()
	if len(keys) != 2 {
		t.Fatalf("redis keys = %v, want one for the email and one for the IP", keys)
	}
}
