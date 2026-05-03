package httpapi

import (
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/auth/middleware"
	"github.com/S-martin-7/sms/internal/events"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/S-martin-7/sms/internal/webhooks"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type RouterDeps struct {
	AdminSvc              *admin.Service
	TenancySvc            *tenancy.Service
	SMSSvc                *sms.Service
	WebhooksSvc           *webhooks.Service
	EventsSvc             *events.Service
	BalanceCache          *BalanceCache // nil → /v1/balance returns 503
	Pool                  *pgxpool.Pool // for ad-hoc admin queries (stats)
	JWTSecret             []byte
	JWTTTL                time.Duration
	APIKeyPepper          string
	HorisenCallbackUser   string
	HorisenCallbackPass   string
	HorisenCallbackSecret string // for ?sig= query-string auth (legacy / Horisen default)
	Logger                *zerolog.Logger

	// Per-tenant token-bucket on POST /v1/sms{,/bulk}. Default
	// 5 req/s / burst 10 — see config.SMSPerTenant*.
	SMSPerTenantTPS   float64
	SMSPerTenantBurst int
}

// NewRouter mounts /admin/login, /v1/ping, /v1/sms* and /v1/horisen/* routes.
func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestID)

	r.Use(httpx.SecurityHeaders)

	// Login is brute-force-able by definition (email+password). Wrap the
	// endpoint with a per-IP sliding-window limiter — 5 attempts/minute.
	loginLimiter := httpx.NewLoginRateLimiter(5, 60*time.Second)
	r.With(loginLimiter.Wrap).Post("/admin/login", LoginHandler(d.AdminSvc, d.JWTSecret, d.JWTTTL))

	// Admin API — JWT-protected dashboard backend.
	r.Group(func(r chi.Router) {
		r.Use(middleware.AdminJWT(d.JWTSecret))

		r.Get("/admin/tenants", AdminListTenantsHandler(d.TenancySvc))
		r.Post("/admin/tenants", AdminCreateTenantHandler(d.TenancySvc, d.AdminSvc))
		r.Get("/admin/tenants/{id}", AdminGetTenantHandler(d.TenancySvc))
		r.Post("/admin/tenants/{id}/suspend", AdminSetTenantStatusHandler(d.TenancySvc, d.AdminSvc, "suspended"))
		r.Post("/admin/tenants/{id}/activate", AdminSetTenantStatusHandler(d.TenancySvc, d.AdminSvc, "active"))
		r.Put("/admin/tenants/{id}/allowed-senders", AdminSetTenantAllowedSendersHandler(d.TenancySvc, d.AdminSvc))

		r.Get("/admin/tenants/{id}/api-keys", AdminListAPIKeysHandler(d.TenancySvc))
		r.Post("/admin/tenants/{id}/api-keys", AdminIssueAPIKeyHandler(d.TenancySvc, d.AdminSvc, d.APIKeyPepper))
		r.Post("/admin/api-keys/{id}/revoke", AdminRevokeAPIKeyHandler(d.TenancySvc, d.AdminSvc))

		r.Get("/admin/messages", AdminListMessagesHandler(d.SMSSvc))
		r.Get("/admin/stats", AdminStatsHandler(d.Pool))

		// Calling admin's own profile + 2FA enrollment.
		r.Get("/admin/me", MeHandler(d.AdminSvc))
		r.Post("/admin/me/totp/setup", TOTPSetupHandler(d.AdminSvc))
		r.Post("/admin/me/totp/enable", TOTPEnableHandler(d.AdminSvc))
		r.Post("/admin/me/totp/disable", TOTPDisableHandler(d.AdminSvc))

		r.Get("/admin/tenants/{id}/webhooks", AdminListEndpointsForTenantHandler(d.WebhooksSvc))
		r.Get("/admin/tenants/{id}/webhook-deliveries", AdminListDeliveriesHandler(d.WebhooksSvc))
		r.Post("/admin/webhook-deliveries/{id}/retry", AdminRetryDeliveryHandler(d.WebhooksSvc, d.AdminSvc))

		r.Get("/admin/inbound-numbers", AdminListInboundNumbersHandler(d.SMSSvc))
		r.Post("/admin/inbound-numbers", AdminAssignInboundNumberHandler(d.SMSSvc, d.AdminSvc))
		r.Delete("/admin/inbound-numbers/{msisdn}", AdminUnassignInboundNumberHandler(d.SMSSvc, d.AdminSvc))

		// Contactos + listas (CRUD per-tenant + import CSV).
		r.Get("/admin/tenants/{id}/contacts", AdminListContactsHandler(d.Pool))
		r.Post("/admin/tenants/{id}/contacts", AdminCreateContactHandler(d.Pool, d.AdminSvc))
		r.Post("/admin/tenants/{id}/contacts/import", AdminImportContactsCSVHandler(d.Pool, d.AdminSvc))
		r.Post("/admin/contacts/{id}/opt-out", AdminContactOptOutHandler(d.Pool, d.AdminSvc))
		r.Delete("/admin/contacts/{id}", AdminDeleteContactHandler(d.Pool, d.AdminSvc))
		r.Get("/admin/tenants/{id}/contact-lists", AdminListContactListsHandler(d.Pool))
		r.Post("/admin/tenants/{id}/contact-lists", AdminCreateContactListHandler(d.Pool, d.AdminSvc))
		r.Delete("/admin/contact-lists/{id}", AdminDeleteContactListHandler(d.Pool, d.AdminSvc))
		r.Post("/admin/contact-lists/{id}/members", AdminAddContactsToListHandler(d.Pool, d.AdminSvc))

		// Reportes per-tenant con time series.
		r.Get("/admin/tenants/{id}/report", AdminTenantReportHandler(d.Pool))

		// Programación de envíos (one-shot o weekly).
		r.Get("/admin/tenants/{id}/scheduled", AdminListScheduledHandler(d.Pool))
		r.Post("/admin/tenants/{id}/scheduled", AdminCreateScheduledHandler(d.Pool, d.AdminSvc))
		r.Post("/admin/tenants/{id}/scheduled/import", AdminImportScheduledXLSXHandler(d.Pool, d.AdminSvc))
		r.Post("/admin/scheduled/{id}/pause", AdminPauseScheduledHandler(d.Pool, d.AdminSvc))
		r.Delete("/admin/scheduled/{id}", AdminDeleteScheduledHandler(d.Pool, d.AdminSvc))
	})

	tenantSMSLimiter := httpx.NewTenantSMSLimiter(d.SMSPerTenantTPS, d.SMSPerTenantBurst)
	r.Group(func(r chi.Router) {
		r.Use(middleware.APIKey(d.TenancySvc, d.APIKeyPepper))
		r.Get("/v1/ping", PingHandler())
		// Send paths get the per-tenant token bucket on top of the
		// API-key middleware. GETs are cheap, no need to throttle here.
		r.With(tenantSMSLimiter.Wrap).Post("/v1/sms", SendSMSHandler(d.SMSSvc))
		r.With(tenantSMSLimiter.Wrap).Post("/v1/sms/bulk", SendBulkSMSHandler(d.SMSSvc))
		r.Get("/v1/sms", ListSMSHandler(d.SMSSvc))
		r.Get("/v1/sms/{id}", GetSMSHandler(d.SMSSvc))
		r.Get("/v1/events", ListEventsHandler(d.EventsSvc))
		r.Get("/v1/balance", BalanceHandler(d.BalanceCache))
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
		r.Post("/v1/horisen/dlr", DLRHandler(d.SMSSvc, d.WebhooksSvc, d.EventsSvc, d.Logger))
		r.Post("/v1/horisen/mo", MOHandler(d.SMSSvc, d.WebhooksSvc, d.EventsSvc, d.Logger))
	})

	return r
}
