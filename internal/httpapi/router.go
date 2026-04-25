package httpapi

import (
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/auth/middleware"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/S-martin-7/sms/internal/webhooks"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type RouterDeps struct {
	AdminSvc              *admin.Service
	TenancySvc            *tenancy.Service
	SMSSvc                *sms.Service
	WebhooksSvc           *webhooks.Service
	JWTSecret             []byte
	JWTTTL                time.Duration
	APIKeyPepper          string
	HorisenCallbackUser   string
	HorisenCallbackPass   string
	HorisenCallbackSecret string // for ?sig= query-string auth (legacy / Horisen default)
	Logger                *zerolog.Logger
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
		r.Post("/v1/webhooks", CreateWebhookHandler(d.WebhooksSvc))
		r.Get("/v1/webhooks", ListWebhooksHandler(d.WebhooksSvc))
		r.Get("/v1/webhooks/{id}", GetWebhookHandler(d.WebhooksSvc))
		r.Delete("/v1/webhooks/{id}", DeleteWebhookHandler(d.WebhooksSvc))
	})

	// Horisen callback endpoints — accept EITHER HTTP Basic Auth (panel option
	// "Use HTTP Basic Authentication") OR ?sig=<secret> query string (the
	// historical / default Horisen format).
	r.Group(func(r chi.Router) {
		r.Use(middleware.HorisenCallbackAuth("horisen", middleware.HorisenCallbackAuthConfig{
			BasicUser:   d.HorisenCallbackUser,
			BasicPass:   d.HorisenCallbackPass,
			QuerySecret: d.HorisenCallbackSecret,
		}))
		r.Post("/v1/horisen/dlr", DLRHandler(d.SMSSvc, d.WebhooksSvc, d.Logger))
		r.Post("/v1/horisen/mo", MOHandler(d.SMSSvc, d.WebhooksSvc, d.Logger))
	})

	return r
}
