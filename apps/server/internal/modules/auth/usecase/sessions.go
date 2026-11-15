package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/domain"
)

// Refresh trades a refresh token for a new pair (AUTH-03). The old token is
// revoked in the same transaction that stores the new one.
//
// A token that was already used is a sign of theft: every token of its device
// is revoked, and so is the device, so its access tokens stop at once too. That
// revocation must survive the 401 answer, so it is committed and the error is
// returned after the transaction. An expired token is not a sign of theft and
// only fails.
func (s *Service) Refresh(ctx context.Context, token string) (Session, error) {
	var (
		out    Session
		reused bool
	)
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		now := s.now()
		rt, err := s.repo.RefreshTokenForUpdate(ctx, hashToken(token))
		if notFound(err) {
			return errBadRefresh
		}
		if err != nil {
			return err
		}
		if rt.RevokedAt != nil {
			reused = true
			if err := s.repo.RevokeDeviceRefreshTokens(ctx, rt.DeviceID, now); err != nil {
				return err
			}
			return s.repo.RevokeDeviceByID(ctx, rt.DeviceID, now)
		}
		if !now.Before(rt.ExpiresAt) {
			return errBadRefresh
		}
		d, err := s.repo.ActiveDevice(ctx, rt.DeviceID)
		if notFound(err) {
			return errBadRefresh
		}
		if err != nil {
			return err
		}
		u, err := s.repo.UserByID(ctx, d.UserID)
		if err != nil {
			return err
		}
		if err := s.repo.RevokeRefreshToken(ctx, rt.ID, now); err != nil {
			return err
		}
		if err := s.repo.TouchDevice(ctx, d.ID, now); err != nil {
			return err
		}
		out, err = s.issue(ctx, u, d, now)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	if reused {
		return Session{}, errBadRefresh
	}
	return out, nil
}

// Logout ends the caller's device, or every device of the user (AUTH-04).
func (s *Service) Logout(ctx context.Context, id app.Identity, allDevices bool) error {
	return s.tx.Do(ctx, func(ctx context.Context) error {
		now := s.now()
		if allDevices {
			if err := s.repo.RevokeUserRefreshTokens(ctx, id.UserID, now); err != nil {
				return err
			}
			return s.repo.RevokeUserDevices(ctx, id.UserID, now)
		}
		if err := s.repo.RevokeDeviceRefreshTokens(ctx, id.DeviceID, now); err != nil {
			return err
		}
		return s.repo.RevokeDeviceByID(ctx, id.DeviceID, now)
	})
}

// Devices lists the user's devices that are still logged in (AUTH-05).
func (s *Service) Devices(ctx context.Context, userID string) ([]domain.Device, error) {
	return s.repo.ActiveDevices(ctx, userID)
}

// RevokeDevice logs one device out (AUTH-05). A device of another user, an
// unknown one and one that is already revoked are all app.ErrNotFound.
func (s *Service) RevokeDevice(ctx context.Context, userID, deviceID string) error {
	if _, err := uuid.Parse(deviceID); err != nil {
		return errDeviceNotFound
	}
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		now := s.now()
		if err := s.repo.RevokeDevice(ctx, userID, deviceID, now); err != nil {
			return err
		}
		return s.repo.RevokeDeviceRefreshTokens(ctx, deviceID, now)
	})
	if errors.Is(err, app.ErrNotFound) {
		return errDeviceNotFound
	}
	if err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	return nil
}
