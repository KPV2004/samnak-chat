package httpx

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// apiPrefix is where the router passed to NewAPI is mounted. It is published
// as the OpenAPI server URL, so spec paths stay relative to it ("/meta").
const apiPrefix = "/api/v1"

func init() {
	// Huma builds every error response through NewError; replace it so Huma
	// endpoints use the same error JSON as WriteError.
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		return newAPIError(status, msg, errs...)
	}
}

// apiError is the error JSON: {"error": {"code": ..., "message": ...}}.
type apiError struct {
	status int
	Err    apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Code    string `json:"code" doc:"Machine-readable error code" example:"not_found"`
	Message string `json:"message" doc:"Human-readable description" example:"channel not found"`
}

func (e *apiError) GetStatus() int { return e.status }
func (e *apiError) Error() string  { return e.Err.Message }

func newAPIError(status int, msg string, errs ...error) *apiError {
	// Huma reports validation failures as 422; the error contract says 400.
	if status == http.StatusUnprocessableEntity {
		status = http.StatusBadRequest
	}
	details := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			details = append(details, err.Error())
		}
	}
	if len(details) > 0 {
		msg += ": " + strings.Join(details, "; ")
	}
	return &apiError{status: status, Err: apiErrorBody{Code: errorCode(status), Message: msg}}
}

func errorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case StatusClientClosedRequest:
		return codeClientClosedRequest
	default:
		return "internal"
	}
}

// NewAPI returns a Huma API that serves under r, which the caller mounts at
// /api/v1. It also serves the spec at /openapi.yaml; the built-in docs page
// and schema routes are off.
func NewAPI(r chi.Router, version string) huma.API {
	cfg := huma.DefaultConfig("Samnak Chat API", version)
	cfg.Servers = []*huma.Server{{URL: apiPrefix}}
	cfg.DocsPath = ""
	cfg.SchemasPath = ""
	// Without this Huma adds a "$schema" property to every response body.
	cfg.CreateHooks = nil
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	}
	return humachi.New(r, cfg)
}

// MetaOutput is the response of GET /api/v1/meta.
type MetaOutput struct {
	Body MetaBody
}

// MetaBody tells clients which server they talk to and which modules are on.
type MetaBody struct {
	Name    string   `json:"name" example:"samnak-chat"`
	Version string   `json:"version" example:"0.0.0-dev"`
	Modules []string `json:"modules" nullable:"false" doc:"Enabled modules in startup order" example:"[\"auth\",\"chat\"]"`
}

// RegisterMeta adds GET /meta. modules is the enabled modules in startup order.
func RegisterMeta(api huma.API, name, version string, modules []string) {
	body := MetaBody{Name: name, Version: version, Modules: append([]string{}, modules...)}
	huma.Register(api, huma.Operation{
		OperationID: "get-meta",
		Method:      http.MethodGet,
		Path:        "/meta",
		Summary:     "Server name, version and enabled modules",
		Tags:        []string{"platform"},
	}, func(context.Context, *struct{}) (*MetaOutput, error) {
		return &MetaOutput{Body: body}, nil
	})
}
