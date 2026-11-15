// Package jwt signs and verifies access tokens (JWT, HS256).
package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/usecase"
)

// Tokens implements usecase.AccessTokens. Claims: sub (user ID), did (device
// ID), iat, exp and jti.
type Tokens struct{ secret []byte }

var _ usecase.AccessTokens = (*Tokens)(nil)

// New returns a signer for secret. An empty secret can neither sign nor
// verify: an HMAC with an empty key is forgeable by anyone.
func New(secret []byte) *Tokens { return &Tokens{secret: secret} }

type claims struct {
	DeviceID string `json:"did"`
	gojwt.RegisteredClaims
}

func (t *Tokens) Issue(c usecase.Claims, now time.Time, ttl time.Duration) (string, error) {
	if len(t.secret) == 0 {
		return "", errors.New("jwt: signing secret is not configured")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("jwt: jti: %w", err)
	}
	tok := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims{
		DeviceID: c.DeviceID,
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   c.UserID,
			ID:        hex.EncodeToString(id[:]),
			IssuedAt:  gojwt.NewNumericDate(now),
			ExpiresAt: gojwt.NewNumericDate(now.Add(ttl)),
		},
	})
	s, err := tok.SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("jwt: sign: %w", err)
	}
	return s, nil
}

func (t *Tokens) Parse(token string, now time.Time) (usecase.Claims, error) {
	if len(t.secret) == 0 {
		return usecase.Claims{}, fmt.Errorf("jwt: no secret: %w", app.ErrUnauthorized)
	}
	var c claims
	_, err := gojwt.ParseWithClaims(token, &c, func(*gojwt.Token) (any, error) { return t.secret, nil },
		gojwt.WithValidMethods([]string{gojwt.SigningMethodHS256.Alg()}),
		gojwt.WithExpirationRequired(),
		gojwt.WithTimeFunc(func() time.Time { return now }),
	)
	if err != nil || c.Subject == "" || c.DeviceID == "" {
		return usecase.Claims{}, fmt.Errorf("jwt: invalid token: %w", app.ErrUnauthorized)
	}
	return usecase.Claims{UserID: c.Subject, DeviceID: c.DeviceID}, nil
}
