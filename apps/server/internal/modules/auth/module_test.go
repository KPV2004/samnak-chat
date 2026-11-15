package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
)

func TestRegister(t *testing.T) {
	e := newEnv(t)
	email := uniqueEmail()
	r := e.do("POST", "/auth/register", req{Body: map[string]any{
		"email": "  " + strings.ToUpper(email) + " ", "password": "correct horse", "display_name": "  Alice ",
	}})
	if r.Status != 201 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	j := r.JSON()
	if j["email"] != email || j["display_name"] != "Alice" {
		t.Errorf("email and display_name are not normalised: %v", j)
	}
	if j["locale"] != "th" || j["timezone"] != "Asia/Bangkok" || j["avatar_url"] != "" {
		t.Errorf("defaults = %v", j)
	}
	if len(r.String("id")) != 36 || r.String("id")[14] != '7' {
		t.Errorf("id = %q, want a UUIDv7", r.String("id"))
	}
	created, err := time.Parse(time.RFC3339, r.String("created_at"))
	if err != nil || created.Location() != time.UTC {
		t.Errorf("created_at = %q, want RFC 3339 UTC", r.String("created_at"))
	}
	for k := range j {
		if strings.Contains(k, "password") {
			t.Errorf("response leaks %q", k)
		}
	}
	if strings.Contains(string(r.Body), "argon2") {
		t.Errorf("response leaks the hash: %s", r.Body)
	}
}

func TestRegisterStoresArgon2idHash(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	var hash string
	if err := e.pool.QueryRow(context.Background(), "SELECT password_hash FROM auth_users WHERE email = $1", a.Email).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, a.Password) {
		t.Errorf("stored hash = %q", hash)
	}
}

func TestRegisterDuplicateEmailIsConflict(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	r := e.do("POST", "/auth/register", req{Body: map[string]any{
		"email": strings.ToUpper(a.Email), "password": "another password", "display_name": "Mallory",
	}})
	if r.Status != 409 || r.String("error", "code") != "conflict" {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("x", 129)
	cases := map[string]map[string]any{
		"short password":   {"email": uniqueEmail(), "password": "1234567", "display_name": "A"},
		"long password":    {"email": uniqueEmail(), "password": long, "display_name": "A"},
		"bad email":        {"email": "not-an-email", "password": "12345678", "display_name": "A"},
		"empty email":      {"email": "", "password": "12345678", "display_name": "A"},
		"empty name":       {"email": uniqueEmail(), "password": "12345678", "display_name": ""},
		"blank name":       {"email": uniqueEmail(), "password": "12345678", "display_name": "   "},
		"missing password": {"email": uniqueEmail(), "display_name": "A"},
	}
	for name, body := range cases {
		r := e.do("POST", "/auth/register", req{Body: body})
		if r.Status != 400 || r.String("error", "code") != "invalid" {
			t.Errorf("%s: status = %d: %s", name, r.Status, r.Body)
		}
	}
}

func TestLoginIssuesTokenPair(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	r := e.do("POST", "/auth/login", req{Body: map[string]any{
		"email": strings.ToUpper(a.Email), "password": a.Password, "device_name": "Firefox on Linux", "platform": "web",
	}})
	if r.Status != 200 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	j := r.JSON()
	if j["expires_in"] != float64(900) {
		t.Errorf("expires_in = %v, want 900", j["expires_in"])
	}
	if r.String("access_token") == "" || r.String("refresh_token") == "" || r.String("device_id") == "" {
		t.Fatalf("incomplete token pair: %s", r.Body)
	}
	if r.String("user", "email") != a.Email || r.String("user", "display_name") != "Alice" {
		t.Errorf("user = %v", j["user"])
	}

	var m gojwt.MapClaims
	if _, _, err := gojwt.NewParser().ParseUnverified(r.String("access_token"), &m); err != nil {
		t.Fatal(err)
	}
	if m["sub"] != r.String("user", "id") || m["did"] != r.String("device_id") {
		t.Errorf("claims = %v", m)
	}
	exp, _ := m["exp"].(float64)
	iat, _ := m["iat"].(float64)
	if exp-iat != 900 {
		t.Errorf("exp - iat = %v, want 900", exp-iat)
	}
}

func TestLoginRejectsInvalidInput(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	base := func() map[string]any {
		return map[string]any{"email": a.Email, "password": a.Password, "device_name": "Laptop", "platform": "web"}
	}
	for name, mut := range map[string]func(map[string]any){
		"bad platform":      func(m map[string]any) { m["platform"] = "symbian" },
		"missing platform":  func(m map[string]any) { delete(m, "platform") },
		"empty device name": func(m map[string]any) { m["device_name"] = "" },
		"long device name":  func(m map[string]any) { m["device_name"] = strings.Repeat("d", 101) },
	} {
		b := base()
		mut(b)
		if r := e.do("POST", "/auth/login", req{Body: b}); r.Status != 400 {
			t.Errorf("%s: status = %d: %s", name, r.Status, r.Body)
		}
	}
}

func TestWrongEmailAndWrongPasswordLookTheSame(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	body := func(email, pw string) map[string]any {
		return map[string]any{"email": email, "password": pw, "device_name": "x", "platform": "web"}
	}
	wrongPw := e.do("POST", "/auth/login", req{Body: body(a.Email, "wrong password"), IP: "198.51.100.1"})
	wrongEmail := e.do("POST", "/auth/login", req{Body: body(uniqueEmail(), a.Password), IP: "198.51.100.2"})
	if wrongPw.Status != 401 || wrongEmail.Status != 401 {
		t.Fatalf("statuses = %d, %d", wrongPw.Status, wrongEmail.Status)
	}
	if string(wrongPw.Body) != string(wrongEmail.Body) {
		t.Errorf("bodies differ:\n%s\n%s", wrongPw.Body, wrongEmail.Body)
	}
}

func TestLoginRateLimitPerEmail(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	bad := map[string]any{"email": a.Email, "password": "wrong password", "device_name": "x", "platform": "web"}
	good := map[string]any{"email": a.Email, "password": a.Password, "device_name": "x", "platform": "web"}

	for i := range 5 {
		// Each attempt comes from another IP, so only the email counts.
		ip := "203.0.113." + string(rune('1'+i))
		if r := e.do("POST", "/auth/login", req{Body: bad, IP: ip}); r.Status != 401 {
			t.Fatalf("attempt %d: status = %d", i+1, r.Status)
		}
	}
	r := e.do("POST", "/auth/login", req{Body: good, IP: "203.0.113.99"})
	if r.Status != 429 || r.String("error", "code") != "rate_limited" {
		t.Fatalf("6th attempt: status = %d: %s", r.Status, r.Body)
	}

	e.clock.Advance(61 * time.Second)
	if r := e.do("POST", "/auth/login", req{Body: good, IP: "203.0.113.99"}); r.Status != 200 {
		t.Fatalf("after the window: status = %d: %s", r.Status, r.Body)
	}
}

func TestLoginRateLimitPerIP(t *testing.T) {
	e := newEnv(t)
	for i := range 5 {
		body := map[string]any{"email": uniqueEmail(), "password": "whatever1", "device_name": "x", "platform": "web"}
		if r := e.do("POST", "/auth/login", req{Body: body, IP: "198.51.100.50"}); r.Status != 401 {
			t.Fatalf("attempt %d: status = %d", i+1, r.Status)
		}
	}
	a := e.register("Bob")
	body := map[string]any{"email": a.Email, "password": a.Password, "device_name": "x", "platform": "web"}
	if r := e.do("POST", "/auth/login", req{Body: body, IP: "198.51.100.50"}); r.Status != 429 {
		t.Fatalf("same IP: status = %d: %s", r.Status, r.Body)
	}
	if r := e.do("POST", "/auth/login", req{Body: body, IP: "198.51.100.51"}); r.Status != 200 {
		t.Fatalf("other IP: status = %d: %s", r.Status, r.Body)
	}
}

func TestSuccessfulLoginsAreNotCounted(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	for i := range 8 {
		e.login(a, "device "+string(rune('a'+i)))
	}
}

func TestMe(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	s := e.login(a, "Laptop")

	r := e.do("GET", "/auth/me", req{Token: s.Access})
	if r.Status != 200 || r.String("email") != a.Email || r.String("display_name") != "Alice" {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
}

func TestMeRejectsBadTokens(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	s := e.login(a, "Laptop")

	tampered := s.Access[:len(s.Access)-2] + "xx"
	if s.Access[len(s.Access)-2:] == "xx" {
		tampered = s.Access[:len(s.Access)-2] + "yy"
	}
	parts := strings.Split(s.Access, ".")
	otherKey, _ := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{
		"sub": "x", "did": s.DeviceID, "exp": e.clock.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("not the server secret"))

	for name, tok := range map[string]string{
		"none":          "",
		"garbage":       "garbage",
		"tampered":      tampered,
		"payload swap":  parts[0] + "." + parts[1] + "A." + parts[2],
		"foreign key":   otherKey,
		"refresh token": s.Refresh,
	} {
		if r := e.do("GET", "/auth/me", req{Token: tok}); r.Status != 401 {
			t.Errorf("%s: status = %d: %s", name, r.Status, r.Body)
		}
	}

	e.clock.Advance(15*time.Minute + time.Second)
	if r := e.do("GET", "/auth/me", req{Token: s.Access}); r.Status != 401 {
		t.Errorf("expired: status = %d: %s", r.Status, r.Body)
	}
}

func TestUpdateMe(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	s := e.login(a, "Laptop")

	r := e.do("PATCH", "/auth/me", req{Token: s.Access, Body: map[string]any{
		"display_name": " Alice B ", "avatar_url": "https://cdn.example.com/a.png", "locale": "en", "timezone": "Europe/London",
	}})
	if r.Status != 200 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	for k, want := range map[string]string{
		"display_name": "Alice B", "avatar_url": "https://cdn.example.com/a.png", "locale": "en", "timezone": "Europe/London",
	} {
		if r.String(k) != want {
			t.Errorf("%s = %q, want %q", k, r.String(k), want)
		}
	}

	// Fields that are not sent stay as they are; an empty avatar clears it.
	r = e.do("PATCH", "/auth/me", req{Token: s.Access, Body: map[string]any{"avatar_url": ""}})
	if r.Status != 200 || r.String("avatar_url") != "" || r.String("display_name") != "Alice B" || r.String("locale") != "en" {
		t.Fatalf("partial patch: %d %s", r.Status, r.Body)
	}
	if r := e.do("GET", "/auth/me", req{Token: s.Access}); r.String("display_name") != "Alice B" {
		t.Errorf("not persisted: %s", r.Body)
	}
	if r := e.do("PATCH", "/auth/me", req{Token: s.Access, Body: map[string]any{}}); r.Status != 200 {
		t.Errorf("empty patch: %d %s", r.Status, r.Body)
	}
}

func TestUpdateMeRejectsInvalidInput(t *testing.T) {
	e := newEnv(t)
	s := e.login(e.register("Alice"), "Laptop")
	for name, body := range map[string]map[string]any{
		"locale":        {"locale": "fr"},
		"timezone":      {"timezone": "Mars/Olympus"},
		"empty tz":      {"timezone": ""},
		"empty name":    {"display_name": ""},
		"avatar scheme": {"avatar_url": "javascript:alert(1)"},
	} {
		if r := e.do("PATCH", "/auth/me", req{Token: s.Access, Body: body}); r.Status != 400 {
			t.Errorf("%s: status = %d: %s", name, r.Status, r.Body)
		}
	}
	if r := e.do("PATCH", "/auth/me", req{Body: map[string]any{"locale": "en"}}); r.Status != 401 {
		t.Errorf("no token: status = %d", r.Status)
	}
}

func TestProvidesAuthenticator(t *testing.T) {
	e := newEnv(t)
	s := e.login(e.register("Alice"), "Laptop")
	authn, ok := app.Lookup[app.Authenticator](e.services)
	if !ok {
		t.Fatal("auth does not provide app.Authenticator")
	}
	id, err := authn.Authenticate(context.Background(), s.Access)
	if err != nil || id.DeviceID != s.DeviceID || id.UserID == "" {
		t.Fatalf("Authenticate = %+v, %v", id, err)
	}
	for _, tok := range []string{"", "junk", s.Refresh} {
		if _, err := authn.Authenticate(context.Background(), tok); !errors.Is(err, app.ErrUnauthorized) {
			t.Errorf("Authenticate(%q) err = %v, want ErrUnauthorized", tok, err)
		}
	}
}

func TestProvidesDirectory(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	b := e.register("Bob")
	s := e.login(a, "Laptop")
	aliceID := e.do("GET", "/auth/me", req{Token: s.Access}).String("id")
	e.login(b, "Phone")

	dir, ok := app.Lookup[auth.Directory](e.services)
	if !ok {
		t.Fatal("auth does not provide auth.Directory")
	}
	got, err := dir.Users(context.Background(), []string{aliceID, "00000000-0000-7000-8000-000000000000", "not-a-uuid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != aliceID || got[0].Email != a.Email || got[0].DisplayName != "Alice" {
		t.Fatalf("Users = %+v", got)
	}
	if got, err := dir.Users(context.Background(), nil); err != nil || len(got) != 0 {
		t.Fatalf("Users(nil) = %+v, %v", got, err)
	}
}

// Sessions: refresh rotation, logout and devices (M1-02).

func (e *env) refresh(token string) resp {
	return e.do("POST", "/auth/refresh", req{Body: map[string]any{"refresh_token": token}})
}

func (e *env) me(access string) int {
	return e.do("GET", "/auth/me", req{Token: access}).Status
}

func TestRefreshRotates(t *testing.T) {
	e := newEnv(t)
	s := e.login(e.register("Alice"), "Laptop")
	e.clock.Advance(time.Minute)

	r := e.refresh(s.Refresh)
	if r.Status != 200 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	if r.String("refresh_token") == "" || r.String("refresh_token") == s.Refresh {
		t.Error("refresh token was not rotated")
	}
	if r.String("access_token") == s.Access || r.String("device_id") != s.DeviceID {
		t.Errorf("access token or device wrong: %s", r.Body)
	}
	if r.JSON()["expires_in"] != float64(900) || r.String("user", "display_name") != "Alice" {
		t.Errorf("body = %s", r.Body)
	}
	if e.me(r.String("access_token")) != 200 {
		t.Error("new access token does not work")
	}

	// The new refresh token works once more; the first is spent.
	if r2 := e.refresh(r.String("refresh_token")); r2.Status != 200 {
		t.Fatalf("second rotation: %d %s", r2.Status, r2.Body)
	}
}

func TestRefreshStoresOnlyTheHash(t *testing.T) {
	e := newEnv(t)
	s := e.login(e.register("Alice"), "Laptop")
	var n int
	err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auth_refresh_tokens WHERE encode(token_hash, 'escape') = $1 OR encode(token_hash, 'base64') LIKE '%' || $1 || '%'", s.Refresh).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("the raw refresh token is stored")
	}
	var size int
	if err := e.pool.QueryRow(context.Background(), "SELECT length(token_hash) FROM auth_refresh_tokens WHERE device_id = $1", s.DeviceID).Scan(&size); err != nil || size != 32 {
		t.Errorf("token_hash length = %d, %v; want a SHA-256", size, err)
	}
}

func TestRefreshReuseRevokesTheDevice(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	s := e.login(a, "Laptop")
	other := e.login(a, "Phone")

	first := e.refresh(s.Refresh)
	if first.Status != 200 {
		t.Fatalf("first refresh: %d %s", first.Status, first.Body)
	}
	// An attacker (or a buggy client) replays the spent token.
	if r := e.refresh(s.Refresh); r.Status != 401 {
		t.Fatalf("replay: status = %d: %s", r.Status, r.Body)
	}
	// The newest token of that device is revoked as well, and so is its access token.
	if r := e.refresh(first.String("refresh_token")); r.Status != 401 {
		t.Errorf("newest token still works after reuse: %d %s", r.Status, r.Body)
	}
	if e.me(first.String("access_token")) != 401 || e.me(s.Access) != 401 {
		t.Error("access tokens of the compromised device still work")
	}
	var live int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auth_refresh_tokens WHERE device_id = $1 AND revoked_at IS NULL", s.DeviceID).Scan(&live); err != nil || live != 0 {
		t.Errorf("%d live refresh tokens remain on the device, %v", live, err)
	}

	// Other devices of the user are untouched.
	if e.me(other.Access) != 200 || e.refresh(other.Refresh).Status != 200 {
		t.Error("another device was revoked")
	}
}

func TestRefreshRejectsUnknownExpiredAndMalformed(t *testing.T) {
	e := newEnv(t)
	s := e.login(e.register("Alice"), "Laptop")
	for name, tok := range map[string]string{"unknown": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "garbage": "x", "access token": s.Access} {
		if r := e.refresh(tok); r.Status != 401 {
			t.Errorf("%s: status = %d: %s", name, r.Status, r.Body)
		}
	}
	if r := e.do("POST", "/auth/refresh", req{Body: map[string]any{}}); r.Status != 400 {
		t.Errorf("missing token: status = %d", r.Status)
	}

	e.clock.Advance(30*24*time.Hour + time.Second)
	if r := e.refresh(s.Refresh); r.Status != 401 {
		t.Errorf("expired: status = %d: %s", r.Status, r.Body)
	}
	// Expiry is not theft: the device is not revoked for it.
	var revoked bool
	if err := e.pool.QueryRow(context.Background(), "SELECT revoked_at IS NOT NULL FROM auth_devices WHERE id = $1", s.DeviceID).Scan(&revoked); err != nil || revoked {
		t.Errorf("device revoked by an expired token: %v, %v", revoked, err)
	}
}

func TestConcurrentRefreshWithOneTokenSucceedsOnce(t *testing.T) {
	e := newEnv(t)
	s := e.login(e.register("Alice"), "Laptop")
	const n = 6
	statuses := make(chan int, n)
	for range n {
		go func() { statuses <- e.refresh(s.Refresh).Status }()
	}
	ok := 0
	for range n {
		if <-statuses == 200 {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d refreshes succeeded with one token, want exactly 1", ok)
	}
}

func TestRefreshUpdatesLastSeen(t *testing.T) {
	e := newEnv(t)
	s := e.login(e.register("Alice"), "Laptop")
	lastSeen := func() time.Time {
		var at time.Time
		if err := e.pool.QueryRow(context.Background(), "SELECT last_seen_at FROM auth_devices WHERE id = $1", s.DeviceID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	atLogin := lastSeen()
	if d := atLogin.Sub(e.clock.Now()); d > time.Second || d < -time.Second {
		t.Errorf("last_seen_at at login = %v, clock = %v", atLogin, e.clock.Now())
	}
	e.clock.Advance(10 * time.Minute)
	if r := e.refresh(s.Refresh); r.Status != 200 {
		t.Fatal(r.Body)
	}
	if got := lastSeen(); got.Sub(atLogin) < 9*time.Minute {
		t.Errorf("last_seen_at after refresh = %v, login = %v", got, atLogin)
	}
}

func TestLogoutRevokesCurrentDevice(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	laptop := e.login(a, "Laptop")
	phone := e.login(a, "Phone")

	if r := e.do("POST", "/auth/logout", req{Token: laptop.Access, Body: map[string]any{"all_devices": false}}); r.Status != 204 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	// Takes effect at once, with the access token still unexpired.
	if e.me(laptop.Access) != 401 {
		t.Error("access token works after logout")
	}
	if r := e.refresh(laptop.Refresh); r.Status != 401 {
		t.Errorf("refresh token works after logout: %d", r.Status)
	}
	if e.me(phone.Access) != 200 || e.refresh(phone.Refresh).Status != 200 {
		t.Error("another device was logged out")
	}
	// Logging out twice is a 401: the token is already dead.
	if r := e.do("POST", "/auth/logout", req{Token: laptop.Access, Body: map[string]any{}}); r.Status != 401 {
		t.Errorf("second logout: status = %d", r.Status)
	}
}

func TestLogoutAllDevices(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	laptop := e.login(a, "Laptop")
	phone := e.login(a, "Phone")
	bob := e.login(e.register("Bob"), "Laptop")

	if r := e.do("POST", "/auth/logout", req{Token: laptop.Access, Body: map[string]any{"all_devices": true}}); r.Status != 204 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	for name, s := range map[string]session{"laptop": laptop, "phone": phone} {
		if e.me(s.Access) != 401 || e.refresh(s.Refresh).Status != 401 {
			t.Errorf("%s still has access", name)
		}
	}
	if e.me(bob.Access) != 200 {
		t.Error("another user was logged out")
	}
}

func TestLogoutRequiresLogin(t *testing.T) {
	e := newEnv(t)
	if r := e.do("POST", "/auth/logout", req{Body: map[string]any{}}); r.Status != 401 {
		t.Errorf("status = %d", r.Status)
	}
}

func TestDevices(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	laptop := e.login(a, "Laptop")
	e.clock.Advance(time.Minute)
	phone := e.login(a, "Phone")
	e.login(e.register("Bob"), "Bob's laptop")

	r := e.do("GET", "/auth/devices", req{Token: laptop.Access})
	if r.Status != 200 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	items, _ := r.JSON()["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %s, want 2 devices of the user only", r.Body)
	}
	byID := map[string]map[string]any{}
	for _, it := range items {
		d := it.(map[string]any)
		byID[d["id"].(string)] = d
	}
	l, p := byID[laptop.DeviceID], byID[phone.DeviceID]
	if l == nil || p == nil {
		t.Fatalf("devices = %s", r.Body)
	}
	if l["name"] != "Laptop" || l["platform"] != "web" || l["current"] != true || p["current"] != false {
		t.Errorf("laptop = %v, phone = %v", l, p)
	}
	if _, err := time.Parse(time.RFC3339, l["last_seen_at"].(string)); err != nil {
		t.Errorf("last_seen_at = %v", l["last_seen_at"])
	}
	// Newest first.
	if items[0].(map[string]any)["id"] != phone.DeviceID {
		t.Errorf("order: %s", r.Body)
	}
	if strings.Contains(string(r.Body), "push_token") {
		t.Errorf("leaks push_token: %s", r.Body)
	}
}

func TestRevokeDevice(t *testing.T) {
	e := newEnv(t)
	a := e.register("Alice")
	laptop := e.login(a, "Laptop")
	phone := e.login(a, "Phone")

	if r := e.do("DELETE", "/auth/devices/"+phone.DeviceID, req{Token: laptop.Access}); r.Status != 204 {
		t.Fatalf("status = %d: %s", r.Status, r.Body)
	}
	if e.me(phone.Access) != 401 || e.refresh(phone.Refresh).Status != 401 {
		t.Error("revoked device still has access")
	}
	if e.me(laptop.Access) != 200 {
		t.Error("current device was revoked")
	}
	r := e.do("GET", "/auth/devices", req{Token: laptop.Access})
	if items, _ := r.JSON()["items"].([]any); len(items) != 1 {
		t.Errorf("revoked device is still listed: %s", r.Body)
	}
	// Already revoked: not found.
	if r := e.do("DELETE", "/auth/devices/"+phone.DeviceID, req{Token: laptop.Access}); r.Status != 404 {
		t.Errorf("second delete: status = %d", r.Status)
	}
}

func TestRevokeAnotherUsersDeviceIsNotFound(t *testing.T) {
	e := newEnv(t)
	alice := e.login(e.register("Alice"), "Laptop")
	bob := e.login(e.register("Bob"), "Laptop")

	for name, id := range map[string]string{
		"another user's": bob.DeviceID,
		"unknown":        "00000000-0000-7000-8000-000000000000",
		"not a uuid":     "nope",
	} {
		if r := e.do("DELETE", "/auth/devices/"+id, req{Token: alice.Access}); r.Status != 404 {
			t.Errorf("%s device: status = %d: %s", name, r.Status, r.Body)
		}
	}
	if e.me(bob.Access) != 200 {
		t.Error("Bob's device was revoked by Alice")
	}
}

func TestSessionEndpointsRequireLogin(t *testing.T) {
	e := newEnv(t)
	for _, c := range [][2]string{{"GET", "/auth/devices"}, {"DELETE", "/auth/devices/x"}} {
		if r := e.do(c[0], c[1], req{}); r.Status != 401 {
			t.Errorf("%s %s: status = %d", c[0], c[1], r.Status)
		}
	}
}
