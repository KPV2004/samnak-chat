package jwt_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/jwt"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/usecase"
)

var (
	secret = []byte("test-secret-test-secret-test-secret")
	now    = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	claims = usecase.Claims{UserID: "user-1", DeviceID: "device-1"}
)

func TestIssueAndParse(t *testing.T) {
	tk := jwt.New(secret)
	tok, err := tk.Issue(claims, now, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tk.Parse(tok, now.Add(14*time.Minute))
	if err != nil || got != claims {
		t.Fatalf("Parse = %+v, %v; want %+v", got, err, claims)
	}
}

func TestIssuedTokenCarriesDocumentedClaims(t *testing.T) {
	tok, _ := jwt.New(secret).Issue(claims, now, 15*time.Minute)
	var m gojwt.MapClaims
	parsed, _, err := gojwt.NewParser().ParseUnverified(tok, &m)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Method.Alg() != "HS256" {
		t.Errorf("alg = %s", parsed.Method.Alg())
	}
	for _, k := range []string{"sub", "did", "iat", "exp", "jti"} {
		if _, ok := m[k]; !ok {
			t.Errorf("claim %q missing: %v", k, m)
		}
	}
	if m["sub"] != "user-1" || m["did"] != "device-1" {
		t.Errorf("claims = %v", m)
	}
	if exp, _ := m["exp"].(float64); int64(exp) != now.Add(15*time.Minute).Unix() {
		t.Errorf("exp = %v", m["exp"])
	}
	other, _ := jwt.New(secret).Issue(claims, now, 15*time.Minute)
	if other == tok {
		t.Error("two tokens are identical: jti is not random")
	}
}

func TestParseRejects(t *testing.T) {
	tk := jwt.New(secret)
	good, _ := tk.Issue(claims, now, 15*time.Minute)

	// Signed with another key.
	forged, _ := jwt.New([]byte("another-secret-another-secret-xx")).Issue(claims, now, 15*time.Minute)
	// alg=none.
	none, _ := gojwt.NewWithClaims(gojwt.SigningMethodNone, gojwt.MapClaims{
		"sub": "u", "did": "d", "exp": now.Add(time.Hour).Unix(),
	}).SignedString(gojwt.UnsafeAllowNoneSignatureType)
	// HS512 with the right key: wrong algorithm.
	hs512, _ := gojwt.NewWithClaims(gojwt.SigningMethodHS512, gojwt.MapClaims{
		"sub": "u", "did": "d", "exp": now.Add(time.Hour).Unix(),
	}).SignedString(secret)
	// Valid signature but no exp, no sub or no did.
	noExp, _ := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{"sub": "u", "did": "d"}).SignedString(secret)
	noSub, _ := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{"did": "d", "exp": now.Add(time.Hour).Unix()}).SignedString(secret)
	noDid, _ := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{"sub": "u", "exp": now.Add(time.Hour).Unix()}).SignedString(secret)

	// Flip one character of the signature.
	i := strings.LastIndex(good, ".") + 1
	flipped := good[:i] + string(rune(good[i]^1)) + good[i+1:]
	// Change the payload but keep the old signature.
	parts := strings.Split(good, ".")
	tampered := parts[0] + "." + parts[1] + "A" + "." + parts[2]

	cases := map[string]struct {
		token string
		at    time.Time
	}{
		"expired":         {good, now.Add(15*time.Minute + time.Second)},
		"exactly expired": {good, now.Add(15 * time.Minute)},
		"wrong key":       {forged, now},
		"alg none":        {none, now},
		"wrong alg":       {hs512, now},
		"no exp":          {noExp, now},
		"no sub":          {noSub, now},
		"no did":          {noDid, now},
		"bad signature":   {flipped, now},
		"tampered":        {tampered, now},
		"empty":           {"", now},
		"garbage":         {"not.a.jwt", now},
	}
	for name, c := range cases {
		if _, err := tk.Parse(c.token, c.at); !errors.Is(err, app.ErrUnauthorized) {
			t.Errorf("%s: err = %v, want ErrUnauthorized", name, err)
		}
	}
}

func TestEmptySecretNeverSigns(t *testing.T) {
	tk := jwt.New(nil)
	if _, err := tk.Issue(claims, now, time.Minute); err == nil {
		t.Error("Issue with an empty secret succeeded")
	}
	// An attacker signing with an empty key must not get through either.
	forged, _ := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{
		"sub": "u", "did": "d", "exp": now.Add(time.Hour).Unix(),
	}).SignedString([]byte{})
	if _, err := tk.Parse(forged, now); !errors.Is(err, app.ErrUnauthorized) {
		t.Errorf("Parse with an empty secret: err = %v, want ErrUnauthorized", err)
	}
}
