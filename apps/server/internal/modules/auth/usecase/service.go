package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/domain"
)

const (
	// AccessTTL is how long an access token lives (AUTH-02).
	AccessTTL = 15 * time.Minute
	// RefreshTTL is how long a refresh token lives (AUTH-02).
	RefreshTTL = 30 * 24 * time.Hour

	defaultLocale   = domain.LocaleThai
	defaultTimezone = "Asia/Bangkok"
	maxDeviceName   = 100
)

// Service holds the auth flows.
type Service struct {
	repo    Repository
	tx      app.TxManager
	limiter LoginLimiter
	tokens  AccessTokens
	events  Events
	now     func() time.Time
}

// Deps are what a Service needs. Now defaults to time.Now and Events defaults
// to a no-op, so a test that does not care about events needs neither.
type Deps struct {
	Repo    Repository
	Tx      app.TxManager
	Limiter LoginLimiter
	Tokens  AccessTokens
	Events  Events
	Now     func() time.Time
}

func NewService(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	events := d.Events
	if events == nil {
		events = noopEvents{}
	}
	return &Service{repo: d.Repo, tx: d.Tx, limiter: d.Limiter, tokens: d.Tokens, events: events, now: now}
}

// noopEvents is used when Deps.Events is nil, so a service always has one.
type noopEvents struct{}

func (noopEvents) DeviceRevoked(context.Context, string, string) {}

// Session is a token pair with the user it belongs to.
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
	DeviceID     string
	User         domain.User
}

type RegisterInput struct {
	Email, Password, DisplayName string
}

type LoginInput struct {
	Email, Password, DeviceName, Platform string
	// IP is the address the request came from, "" when unknown.
	IP string
}

// Register creates an account (AUTH-01).
func (s *Service) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	email, err := domain.NormalizeEmail(in.Email)
	if err != nil {
		return domain.User{}, invalid(err)
	}
	if err := domain.ValidatePassword(in.Password); err != nil {
		return domain.User{}, invalid(err)
	}
	name, err := domain.NormalizeDisplayName(in.DisplayName)
	if err != nil {
		return domain.User{}, invalid(err)
	}
	hash, err := domain.HashPassword(in.Password)
	if err != nil {
		return domain.User{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.User{}, fmt.Errorf("register: new id: %w", err)
	}
	u, err := s.repo.CreateUser(ctx, domain.User{
		ID: id.String(), Email: email, PasswordHash: hash, DisplayName: name,
		Locale: defaultLocale, Timezone: defaultTimezone, CreatedAt: s.now(),
	})
	if errors.Is(err, app.ErrConflict) {
		return domain.User{}, errEmailTaken
	}
	return u, err
}

// Login checks the password and opens a session on a new device (AUTH-02).
// More than five failures per minute for one email or one IP are answered with
// app.ErrRateLimited before the password is looked at (AUTH-06).
func (s *Service) Login(ctx context.Context, in LoginInput) (Session, error) {
	deviceName := strings.TrimSpace(in.DeviceName)
	if n := len([]rune(deviceName)); n < 1 || n > maxDeviceName {
		return Session{}, invalid(errors.New("device name must be 1 to 100 characters"))
	}
	if err := domain.ValidatePlatform(in.Platform); err != nil {
		return Session{}, invalid(err)
	}

	// The email is not validated here: a malformed one is a wrong login, not a
	// reason to answer differently.
	email := strings.ToLower(strings.TrimSpace(in.Email))
	keys := []string{"email:" + email}
	if in.IP != "" {
		keys = append(keys, "ip:"+in.IP)
	}
	for _, k := range keys {
		blocked, err := s.limiter.Blocked(ctx, k)
		if err != nil {
			return Session{}, fmt.Errorf("login: limiter: %w", err)
		}
		if blocked {
			return Session{}, errRateLimited
		}
	}

	u, err := s.repo.UserByEmail(ctx, email)
	switch {
	case notFound(err):
		// Spend the same time as a real check so the response time does not
		// reveal whether the email exists.
		domain.VerifyPassword(dummyHash(), in.Password)
		return Session{}, s.failLogin(ctx, keys)
	case err != nil:
		return Session{}, err
	case !domain.VerifyPassword(u.PasswordHash, in.Password):
		return Session{}, s.failLogin(ctx, keys)
	}

	var out Session
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		now := s.now()
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("login: new id: %w", err)
		}
		d, err := s.repo.CreateDevice(ctx, domain.Device{
			ID: id.String(), UserID: u.ID, Name: deviceName, Platform: in.Platform, LastSeenAt: now, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		out, err = s.issue(ctx, u, d, now)
		return err
	})
	return out, err
}

func (s *Service) failLogin(ctx context.Context, keys []string) error {
	for _, k := range keys {
		if err := s.limiter.Fail(ctx, k); err != nil {
			return fmt.Errorf("login: limiter: %w", err)
		}
	}
	return errBadCredentials
}

// dummyHash is a valid argon2id hash that no password matches.
var dummyHash = sync.OnceValue(func() string {
	h, err := domain.HashPassword("dummy password for timing")
	if err != nil {
		panic(err) // crypto/rand failed; nothing sensible can run
	}
	return h
})

// issue creates and stores a refresh token and signs an access token for d.
func (s *Service) issue(ctx context.Context, u domain.User, d domain.Device, now time.Time) (Session, error) {
	access, err := s.tokens.Issue(Claims{UserID: u.ID, DeviceID: d.ID}, now, AccessTTL)
	if err != nil {
		return Session{}, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, fmt.Errorf("refresh token: %w", err)
	}
	refresh := base64.RawURLEncoding.EncodeToString(raw)
	id, err := uuid.NewV7()
	if err != nil {
		return Session{}, fmt.Errorf("refresh token: new id: %w", err)
	}
	if err := s.repo.CreateRefreshToken(ctx, domain.RefreshToken{
		ID: id.String(), DeviceID: d.ID, TokenHash: hashToken(refresh), ExpiresAt: now.Add(RefreshTTL),
	}); err != nil {
		return Session{}, err
	}
	return Session{AccessToken: access, RefreshToken: refresh, ExpiresIn: AccessTTL, DeviceID: d.ID, User: u}, nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// Authenticate implements app.Authenticator: the token must verify and its
// device must not be revoked, so a logout takes effect at once.
func (s *Service) Authenticate(ctx context.Context, token string) (app.Identity, error) {
	c, err := s.tokens.Parse(token, s.now())
	if err != nil {
		return app.Identity{}, err
	}
	d, err := s.repo.ActiveDevice(ctx, c.DeviceID)
	if notFound(err) || (err == nil && d.UserID != c.UserID) {
		return app.Identity{}, fmt.Errorf("device revoked: %w", app.ErrUnauthorized)
	}
	if err != nil {
		return app.Identity{}, err
	}
	return app.Identity{UserID: c.UserID, DeviceID: c.DeviceID}, nil
}

func (s *Service) Me(ctx context.Context, userID string) (domain.User, error) {
	return s.repo.UserByID(ctx, userID)
}

// UpdateProfile changes the fields set in p (AUTH-07).
func (s *Service) UpdateProfile(ctx context.Context, userID string, p ProfilePatch) (domain.User, error) {
	if p.DisplayName != nil {
		name, err := domain.NormalizeDisplayName(*p.DisplayName)
		if err != nil {
			return domain.User{}, invalid(err)
		}
		p.DisplayName = &name
	}
	if p.AvatarURL != nil {
		if err := domain.ValidateAvatarURL(*p.AvatarURL); err != nil {
			return domain.User{}, invalid(err)
		}
	}
	if p.Locale != nil {
		if err := domain.ValidateLocale(*p.Locale); err != nil {
			return domain.User{}, invalid(err)
		}
	}
	if p.Timezone != nil {
		if err := domain.ValidateTimezone(*p.Timezone); err != nil {
			return domain.User{}, invalid(err)
		}
	}
	return s.repo.UpdateProfile(ctx, userID, p, s.now())
}

// Users returns the users that exist among ids. IDs that are not UUIDs cannot
// exist and are skipped.
func (s *Service) Users(ctx context.Context, ids []string) ([]domain.User, error) {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err == nil {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		return nil, nil
	}
	return s.repo.UsersByIDs(ctx, valid)
}
