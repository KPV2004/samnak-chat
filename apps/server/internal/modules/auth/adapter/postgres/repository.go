// Package postgres implements usecase.Repository with sqlc.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/adapter/postgres/sqlcgen"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

const uniqueViolation = "23505"

// Repository implements usecase.Repository. It uses the transaction in ctx
// when there is one.
type Repository struct{ pool *pgxpool.Pool }

var _ usecase.Repository = (*Repository)(nil)

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(database.Querier(ctx, r.pool))
}

// wrap turns pgx's "no rows" into app.ErrNotFound and keeps other errors.
func wrap(what string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("%s: %w", what, app.ErrNotFound)
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}

func toUser(u sqlcgen.AuthUser) domain.User {
	out := domain.User{
		ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash, DisplayName: u.DisplayName,
		AvatarURL: u.AvatarUrl, Locale: u.Locale, Timezone: u.Timezone, CreatedAt: u.CreatedAt.UTC(),
	}
	if u.EmailVerifiedAt != nil {
		at := u.EmailVerifiedAt.UTC()
		out.EmailVerifiedAt = &at
	}
	return out
}

func toDevice(d sqlcgen.AuthDevice) domain.Device {
	return domain.Device{
		ID: d.ID, UserID: d.UserID, Name: d.Name, Platform: d.Platform,
		LastSeenAt: d.LastSeenAt.UTC(), CreatedAt: d.CreatedAt.UTC(),
	}
}

func (r *Repository) CreateUser(ctx context.Context, u domain.User) (domain.User, error) {
	row, err := r.q(ctx).CreateUser(ctx, sqlcgen.CreateUserParams{
		ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash, DisplayName: u.DisplayName,
		AvatarUrl: u.AvatarURL, Locale: u.Locale, Timezone: u.Timezone, CreatedAt: u.CreatedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.User{}, fmt.Errorf("create user: %w", app.ErrConflict)
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return toUser(row), nil
}

func (r *Repository) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := r.q(ctx).GetUserByEmail(ctx, email)
	if err != nil {
		return domain.User{}, wrap("user by email", err)
	}
	return toUser(row), nil
}

func (r *Repository) UserByID(ctx context.Context, id string) (domain.User, error) {
	row, err := r.q(ctx).GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, wrap("user by id", err)
	}
	return toUser(row), nil
}

func (r *Repository) UsersByIDs(ctx context.Context, ids []string) ([]domain.User, error) {
	rows, err := r.q(ctx).GetUsersByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("users by ids: %w", err)
	}
	out := make([]domain.User, len(rows))
	for i, row := range rows {
		out[i] = toUser(row)
	}
	return out, nil
}

func text(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func (r *Repository) UpdateProfile(ctx context.Context, id string, p usecase.ProfilePatch, now time.Time) (domain.User, error) {
	row, err := r.q(ctx).UpdateUserProfile(ctx, sqlcgen.UpdateUserProfileParams{
		DisplayName: text(p.DisplayName), AvatarUrl: text(p.AvatarURL), Locale: text(p.Locale), Timezone: text(p.Timezone),
		Now: now, ID: id,
	})
	if err != nil {
		return domain.User{}, wrap("update profile", err)
	}
	return toUser(row), nil
}

func (r *Repository) CreateDevice(ctx context.Context, d domain.Device) (domain.Device, error) {
	row, err := r.q(ctx).CreateDevice(ctx, sqlcgen.CreateDeviceParams{
		ID: d.ID, UserID: d.UserID, Name: d.Name, Platform: d.Platform, LastSeenAt: d.LastSeenAt,
	})
	if err != nil {
		return domain.Device{}, fmt.Errorf("create device: %w", err)
	}
	return toDevice(row), nil
}

func (r *Repository) ActiveDevice(ctx context.Context, id string) (domain.Device, error) {
	row, err := r.q(ctx).GetActiveDevice(ctx, id)
	if err != nil {
		return domain.Device{}, wrap("active device", err)
	}
	return toDevice(row), nil
}

func (r *Repository) TouchDevice(ctx context.Context, id string, now time.Time) error {
	if err := r.q(ctx).TouchDevice(ctx, sqlcgen.TouchDeviceParams{ID: id, LastSeenAt: now}); err != nil {
		return fmt.Errorf("touch device: %w", err)
	}
	return nil
}

func (r *Repository) ActiveDevices(ctx context.Context, userID string) ([]domain.Device, error) {
	rows, err := r.q(ctx).ListActiveDevices(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	out := make([]domain.Device, len(rows))
	for i, row := range rows {
		out[i] = toDevice(row)
	}
	return out, nil
}

func (r *Repository) RevokeDevice(ctx context.Context, userID, deviceID string, now time.Time) error {
	n, err := r.q(ctx).RevokeDevice(ctx, sqlcgen.RevokeDeviceParams{ID: deviceID, UserID: userID, RevokedAt: &now})
	if err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("revoke device: %w", app.ErrNotFound)
	}
	return nil
}

func (r *Repository) RevokeDeviceByID(ctx context.Context, deviceID string, now time.Time) error {
	if err := r.q(ctx).RevokeDeviceByID(ctx, sqlcgen.RevokeDeviceByIDParams{ID: deviceID, RevokedAt: &now}); err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	return nil
}

func (r *Repository) RevokeUserDevices(ctx context.Context, userID string, now time.Time) error {
	if err := r.q(ctx).RevokeUserDevices(ctx, sqlcgen.RevokeUserDevicesParams{UserID: userID, RevokedAt: &now}); err != nil {
		return fmt.Errorf("revoke user devices: %w", err)
	}
	return nil
}

func (r *Repository) CreateRefreshToken(ctx context.Context, t domain.RefreshToken) error {
	err := r.q(ctx).CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
		ID: t.ID, DeviceID: t.DeviceID, TokenHash: t.TokenHash, ExpiresAt: t.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (r *Repository) RefreshTokenForUpdate(ctx context.Context, hash []byte) (domain.RefreshToken, error) {
	row, err := r.q(ctx).GetRefreshTokenForUpdate(ctx, hash)
	if err != nil {
		return domain.RefreshToken{}, wrap("refresh token", err)
	}
	t := domain.RefreshToken{ID: row.ID, DeviceID: row.DeviceID, TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt.UTC()}
	if row.RevokedAt != nil {
		at := row.RevokedAt.UTC()
		t.RevokedAt = &at
	}
	return t, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, id string, now time.Time) error {
	if err := r.q(ctx).RevokeRefreshToken(ctx, sqlcgen.RevokeRefreshTokenParams{ID: id, RevokedAt: &now}); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (r *Repository) RevokeDeviceRefreshTokens(ctx context.Context, deviceID string, now time.Time) error {
	if err := r.q(ctx).RevokeDeviceRefreshTokens(ctx, sqlcgen.RevokeDeviceRefreshTokensParams{DeviceID: deviceID, RevokedAt: &now}); err != nil {
		return fmt.Errorf("revoke device tokens: %w", err)
	}
	return nil
}

func (r *Repository) RevokeUserRefreshTokens(ctx context.Context, userID string, now time.Time) error {
	if err := r.q(ctx).RevokeUserRefreshTokens(ctx, sqlcgen.RevokeUserRefreshTokensParams{UserID: userID, RevokedAt: &now}); err != nil {
		return fmt.Errorf("revoke user tokens: %w", err)
	}
	return nil
}

func (r *Repository) UpdatePassword(ctx context.Context, userID, passwordHash string, now time.Time) error {
	err := r.q(ctx).UpdateUserPassword(ctx, sqlcgen.UpdateUserPasswordParams{PasswordHash: passwordHash, Now: now, ID: userID})
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

func (r *Repository) MarkEmailVerified(ctx context.Context, userID string, now time.Time) error {
	err := r.q(ctx).MarkUserEmailVerified(ctx, sqlcgen.MarkUserEmailVerifiedParams{Now: &now, ID: userID})
	if err != nil {
		return fmt.Errorf("mark email verified: %w", err)
	}
	return nil
}

func (r *Repository) CreateEmailToken(ctx context.Context, t domain.EmailToken) error {
	err := r.q(ctx).CreateEmailToken(ctx, sqlcgen.CreateEmailTokenParams{
		ID: t.ID, UserID: t.UserID, Purpose: string(t.Purpose), TokenHash: t.TokenHash, ExpiresAt: t.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("create email token: %w", err)
	}
	return nil
}

func (r *Repository) EmailTokenForUpdate(ctx context.Context, hash []byte, purpose domain.EmailPurpose) (domain.EmailToken, error) {
	row, err := r.q(ctx).GetEmailTokenForUpdate(ctx, sqlcgen.GetEmailTokenForUpdateParams{TokenHash: hash, Purpose: string(purpose)})
	if err != nil {
		return domain.EmailToken{}, wrap("email token", err)
	}
	t := domain.EmailToken{
		ID: row.ID, UserID: row.UserID, Purpose: domain.EmailPurpose(row.Purpose),
		TokenHash: row.TokenHash, ExpiresAt: row.ExpiresAt.UTC(),
	}
	if row.UsedAt != nil {
		at := row.UsedAt.UTC()
		t.UsedAt = &at
	}
	return t, nil
}

func (r *Repository) MarkEmailTokenUsed(ctx context.Context, id string, now time.Time) error {
	err := r.q(ctx).MarkEmailTokenUsed(ctx, sqlcgen.MarkEmailTokenUsedParams{ID: id, UsedAt: &now})
	if err != nil {
		return fmt.Errorf("mark email token used: %w", err)
	}
	return nil
}
