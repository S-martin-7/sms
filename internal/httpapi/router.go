package httpapi

import (
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/auth/middleware"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type RouterDeps struct {
	AdminSvc           *admin.Service
	TenancySvc         *tenancy.Service
	SMSSvc             *sms.Service
	JWTSecret          []byte
	JWTTTL             time.Duration
	APIKeyPepper       string
	HorisenCallbackUser string
	HorisenCallbackPass string
	Logger             *zerolog.Logger
}

// NewRouter mounts /admin/login, /v1/ping, /v1/sms* and /v1/horisen/* routes.
func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestID)

	r.Post("/admin/login", LoginHandler(d.AdminSvc, d.JWTSecret, d.JWTTTL))

	r.Group(func(r chi.Router) {
		r.Use(middleware.APIKey(d.TenancySvc, d.APIKeyPepper))
		r.Get("/v1/ping", PingHandler())
		r.Post("/v1/sms", SendSMSHandler(d.SMSSvc))
		r.Get("/v1/sms/{id}", GetSMSHandler(d.SMSSvc))
	})

	// Horisen callback endpoints — protected by HTTP Basic Auth configured in
	// the Horisen panel ("Use HTTP Basic Authentication" option).
	r.Group(func(r chi.Router) {
		r.Use(middleware.BasicAuth("horisen", d.HorisenCallbackUser, d.HorisenCallbackPass))
		r.Post("/v1/horisen/dlr", DLRStubHandler(d.Logger))
		r.Post("/v1/horisen/mo", MOStubHandler(d.Logger))
	})

	return r
}
