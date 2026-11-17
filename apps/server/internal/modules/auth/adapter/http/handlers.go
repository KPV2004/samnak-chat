// Package httpadapter maps the auth use cases to HTTP. It holds JSON shapes
// and status codes, not business rules.
package httpadapter

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/httpx"
)

const tag = "auth"

// Handlers serves the auth endpoints.
type Handlers struct {
	svc      *usecase.Service
	services *app.Services
	logger   *slog.Logger
}

func NewHandlers(svc *usecase.Service, services *app.Services, logger *slog.Logger) *Handlers {
	return &Handlers{svc: svc, services: services, logger: logger}
}

// UserBody is the User shape of the API.
type UserBody struct {
	ID          string    `json:"id" example:"0192f0c1-7b1e-7c3a-9d1e-5b6f4a3c2d10"`
	Email       string    `json:"email" example:"alice@example.com"`
	DisplayName string    `json:"display_name" example:"Alice"`
	AvatarURL   string    `json:"avatar_url" doc:"Empty when the user has no avatar" example:"https://cdn.example.com/a.png"`
	Locale      string    `json:"locale" enum:"th,en"`
	Timezone    string    `json:"timezone" doc:"IANA name" example:"Asia/Bangkok"`
	CreatedAt   time.Time `json:"created_at"`
}

func toUserBody(u domain.User) UserBody {
	return UserBody{
		ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL,
		Locale: u.Locale, Timezone: u.Timezone, CreatedAt: u.CreatedAt.UTC(),
	}
}

// TokenPairBody is the TokenPair shape of the API.
type TokenPairBody struct {
	AccessToken  string   `json:"access_token" doc:"JWT, send as Authorization: Bearer"`
	RefreshToken string   `json:"refresh_token" doc:"Opaque, single use: every refresh returns a new one"`
	ExpiresIn    int      `json:"expires_in" doc:"Seconds until the access token expires" example:"900"`
	DeviceID     string   `json:"device_id"`
	User         UserBody `json:"user"`
}

func toTokenPair(s usecase.Session) TokenPairBody {
	return TokenPairBody{
		AccessToken: s.AccessToken, RefreshToken: s.RefreshToken, ExpiresIn: int(s.ExpiresIn.Seconds()),
		DeviceID: s.DeviceID, User: toUserBody(s.User),
	}
}

// DeviceBody is the Device shape of the API.
type DeviceBody struct {
	ID         string    `json:"id"`
	Name       string    `json:"name" example:"Firefox on Linux"`
	Platform   string    `json:"platform" enum:"web,desktop,ios,android"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Current    bool      `json:"current" doc:"True for the device that sent this request"`
}

type userOutput struct{ Body UserBody }
type devicesOutput struct {
	Body struct {
		Items []DeviceBody `json:"items" nullable:"false"`
	}
}
type tokenPairOutput struct{ Body TokenPairBody }

type registerInput struct {
	Body struct {
		Email       string `json:"email" maxLength:"254" doc:"Trimmed and lower-cased; unique" example:"alice@example.com"`
		Password    string `json:"password" minLength:"8" maxLength:"128"`
		DisplayName string `json:"display_name" minLength:"1" maxLength:"100" example:"Alice"`
	}
}

type loginInput struct {
	ip   string
	Body struct {
		Email      string `json:"email" maxLength:"254"`
		Password   string `json:"password" maxLength:"128"`
		DeviceName string `json:"device_name" minLength:"1" maxLength:"100" example:"Firefox on Linux"`
		Platform   string `json:"platform" enum:"web,desktop,ios,android"`
	}
}

// Resolve records where the request came from, for the login limit. Only the
// socket address counts: X-Forwarded-For is client input unless a trusted
// proxy sets it, so it is not read here.
func (i *loginInput) Resolve(ctx huma.Context) []error {
	i.ip = limitKey(ctx.RemoteAddr())
	return nil
}

// limitKey returns the host of addr. An IPv6 address is reduced to its /64,
// because one subscriber usually holds a whole /64.
func limitKey(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	ip = ip.Unmap()
	if ip.Is6() {
		return netip.PrefixFrom(ip, 64).Masked().String()
	}
	return ip.String()
}

type refreshInput struct {
	Body struct {
		RefreshToken string `json:"refresh_token" minLength:"1" maxLength:"512"`
	}
}

type logoutInput struct {
	Body struct {
		AllDevices bool `json:"all_devices,omitempty" doc:"Log out every device of the user, not only this one"`
	}
}

type deviceIDInput struct {
	ID string `path:"id" doc:"Device ID"`
}

type updateMeInput struct {
	Body struct {
		DisplayName *string `json:"display_name,omitempty" minLength:"1" maxLength:"100"`
		AvatarURL   *string `json:"avatar_url,omitempty" maxLength:"2048" doc:"http or https URL; empty removes the avatar"`
		Locale      *string `json:"locale,omitempty" enum:"th,en"`
		Timezone    *string `json:"timezone,omitempty" doc:"IANA name" example:"Asia/Bangkok"`
	}
}

// Register adds the endpoints to api, which is the /auth group.
func (h *Handlers) Register(api huma.API) {
	user := huma.Middlewares{httpx.RequireUser(api, h.services)}

	huma.Register(api, huma.Operation{
		OperationID: "auth-register", Method: http.MethodPost, Path: "/register",
		Summary: "Create an account", Tags: []string{tag}, DefaultStatus: http.StatusCreated,
		Errors: []int{http.StatusBadRequest, http.StatusConflict},
	}, h.register)

	huma.Register(api, huma.Operation{
		OperationID: "auth-login", Method: http.MethodPost, Path: "/login",
		Summary: "Log in on a new device", Tags: []string{tag},
		Description: "Five failed logins per minute per email and per IP are answered with 429.",
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests},
	}, h.login)

	huma.Register(api, huma.Operation{
		OperationID: "auth-get-me", Method: http.MethodGet, Path: "/me",
		Summary: "The current user", Tags: []string{tag},
		Security: httpx.BearerAuth, Middlewares: user, Errors: []int{http.StatusUnauthorized},
	}, h.me)

	huma.Register(api, huma.Operation{
		OperationID: "auth-update-me", Method: http.MethodPatch, Path: "/me",
		Summary: "Edit the profile", Tags: []string{tag},
		Security: httpx.BearerAuth, Middlewares: user, Errors: []int{http.StatusBadRequest, http.StatusUnauthorized},
	}, h.updateMe)

	huma.Register(api, huma.Operation{
		OperationID: "auth-refresh", Method: http.MethodPost, Path: "/refresh",
		Summary: "Trade a refresh token for a new token pair", Tags: []string{tag},
		Description: "The refresh token is single use. Sending a spent one revokes the whole device.",
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized},
	}, h.refresh)

	huma.Register(api, huma.Operation{
		OperationID: "auth-logout", Method: http.MethodPost, Path: "/logout",
		Summary: "Log out this device, or all of them", Tags: []string{tag}, DefaultStatus: http.StatusNoContent,
		Security: httpx.BearerAuth, Middlewares: user, Errors: []int{http.StatusUnauthorized},
	}, h.logout)

	huma.Register(api, huma.Operation{
		OperationID: "auth-list-devices", Method: http.MethodGet, Path: "/devices",
		Summary: "Devices that are logged in", Tags: []string{tag},
		Security: httpx.BearerAuth, Middlewares: user, Errors: []int{http.StatusUnauthorized},
	}, h.devices)

	huma.Register(api, huma.Operation{
		OperationID: "auth-revoke-device", Method: http.MethodDelete, Path: "/devices/{id}",
		Summary: "Log one device out", Tags: []string{tag}, DefaultStatus: http.StatusNoContent,
		Security: httpx.BearerAuth, Middlewares: user, Errors: []int{http.StatusUnauthorized, http.StatusNotFound},
	}, h.revokeDevice)
}

func (h *Handlers) register(ctx context.Context, in *registerInput) (*userOutput, error) {
	u, err := h.svc.Register(ctx, usecase.RegisterInput{
		Email: in.Body.Email, Password: in.Body.Password, DisplayName: in.Body.DisplayName,
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &userOutput{Body: toUserBody(u)}, nil
}

func (h *Handlers) login(ctx context.Context, in *loginInput) (*tokenPairOutput, error) {
	s, err := h.svc.Login(ctx, usecase.LoginInput{
		Email: in.Body.Email, Password: in.Body.Password,
		DeviceName: in.Body.DeviceName, Platform: in.Body.Platform, IP: in.ip,
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &tokenPairOutput{Body: toTokenPair(s)}, nil
}

func (h *Handlers) refresh(ctx context.Context, in *refreshInput) (*tokenPairOutput, error) {
	s, err := h.svc.Refresh(ctx, in.Body.RefreshToken)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &tokenPairOutput{Body: toTokenPair(s)}, nil
}

func (h *Handlers) logout(ctx context.Context, in *logoutInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Logout(ctx, app.Identity{UserID: p.UserID, DeviceID: p.DeviceID}, in.Body.AllDevices); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

func (h *Handlers) devices(ctx context.Context, _ *struct{}) (*devicesOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	ds, err := h.svc.Devices(ctx, p.UserID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := &devicesOutput{}
	out.Body.Items = make([]DeviceBody, len(ds))
	for i, d := range ds {
		out.Body.Items[i] = DeviceBody{
			ID: d.ID, Name: d.Name, Platform: d.Platform, LastSeenAt: d.LastSeenAt.UTC(), Current: d.ID == p.DeviceID,
		}
	}
	return out, nil
}

func (h *Handlers) revokeDevice(ctx context.Context, in *deviceIDInput) (*struct{}, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.RevokeDevice(ctx, p.UserID, in.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return nil, nil
}

func (h *Handlers) me(ctx context.Context, _ *struct{}) (*userOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.svc.Me(ctx, p.UserID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &userOutput{Body: toUserBody(u)}, nil
}

func (h *Handlers) updateMe(ctx context.Context, in *updateMeInput) (*userOutput, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.svc.UpdateProfile(ctx, p.UserID, usecase.ProfilePatch{
		DisplayName: in.Body.DisplayName, AvatarURL: in.Body.AvatarURL,
		Locale: in.Body.Locale, Timezone: in.Body.Timezone,
	})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &userOutput{Body: toUserBody(u)}, nil
}

// principal returns the caller put in the context by httpx.RequireUser.
func principal(ctx context.Context) (app.Principal, error) {
	p, ok := app.PrincipalFrom(ctx)
	if !ok {
		return app.Principal{}, huma.Error401Unauthorized("missing credentials")
	}
	return p, nil
}

// fail maps a use case error to an HTTP error. Anything that is not one of the
// standard errors is logged and hidden behind a plain 500.
func (h *Handlers) fail(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, app.ErrInvalid):
		return huma.Error400BadRequest(err.Error())
	case errors.Is(err, app.ErrUnauthorized):
		return huma.Error401Unauthorized(err.Error())
	case errors.Is(err, app.ErrForbidden):
		return huma.Error403Forbidden(err.Error())
	case errors.Is(err, app.ErrNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, app.ErrConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, app.ErrRateLimited):
		return huma.NewError(http.StatusTooManyRequests, err.Error())
	case httpx.ClientAborted(ctx, err):
		return huma.NewError(httpx.StatusClientClosedRequest, "client closed request")
	}
	h.logger.ErrorContext(ctx, "auth request failed", "request_id", httpx.RequestID(ctx), "error", err)
	return huma.Error500InternalServerError("internal error")
}
