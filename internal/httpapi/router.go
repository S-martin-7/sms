package httpapi

import (
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/auth/middleware"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/go-chi/chi/v5"
)

type RouterDeps struct {
	AdminSvc     *admin.Service
	TenancySvc   *tenancy.Service
	JWTSecret    []byte
	JWTTTL       time.Duration
	APIKeyPepper string
}

// NewRouter mounts /admin/login and /v1/ping.
func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestID)

	r.Post("/admin/login", LoginHandler(d.AdminSvc, d.JWTSecret, d.JWTTTL))

	r.Group(func(r chi.Router) {
		r.Use(middleware.APIKey(d.TenancySvc, d.APIKeyPepper))
		r.Get("/v1/ping", PingHandler())
	})

	return r
}
