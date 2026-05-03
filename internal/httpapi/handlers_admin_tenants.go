package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/tenancy"
	"github.com/go-chi/chi/v5"
)

type adminTenantResp struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	DailySMSLimit  *int32    `json:"daily_sms_limit,omitempty"`
	AllowedSenders []string  `json:"allowed_senders"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func toAdminTenantResp(t *tenancy.Tenant) adminTenantResp {
	senders := t.AllowedSenders
	if senders == nil {
		senders = []string{}
	}
	return adminTenantResp{
		ID:             t.ID,
		Name:           t.Name,
		Status:         t.Status,
		DailySMSLimit:  t.DailySMSLimit,
		AllowedSenders: senders,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}

// AdminListTenantsHandler — GET /admin/tenants
func AdminListTenantsHandler(svc *tenancy.Service) http.HandlerFunc {
	type resp struct {
		Tenants []adminTenantResp `json:"tenants"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := svc.ListTenants(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "list failed")
			return
		}
		out := resp{Tenants: make([]adminTenantResp, 0, len(rows))}
		for _, t := range rows {
			out.Tenants = append(out.Tenants, toAdminTenantResp(t))
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// AdminCreateTenantHandler — POST /admin/tenants
func AdminCreateTenantHandler(svc *tenancy.Service, audit *admin.Service) http.HandlerFunc {
	type req struct {
		Name          string `json:"name"`
		DailySMSLimit *int32 `json:"daily_sms_limit,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		t, err := svc.CreateTenant(r.Context(), tenancy.CreateTenantInput{
			Name:          in.Name,
			DailySMSLimit: in.DailySMSLimit,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"tenant.create", "tenant", strconv.FormatInt(t.ID, 10),
			map[string]any{"name": t.Name})
		httpx.WriteJSON(w, http.StatusCreated, toAdminTenantResp(t))
	}
}

// AdminGetTenantHandler — GET /admin/tenants/{id}
func AdminGetTenantHandler(svc *tenancy.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		t, err := svc.GetTenant(r.Context(), id)
		if errors.Is(err, tenancy.ErrTenantNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "no such tenant")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "get failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toAdminTenantResp(t))
	}
}

// AdminSetTenantStatusHandler — POST /admin/tenants/{id}/suspend or /activate
// Captures the target status from the URL (chi sets it via the route map).
func AdminSetTenantStatusHandler(svc *tenancy.Service, audit *admin.Service, target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		if err := svc.SetStatus(r.Context(), id, target); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"tenant."+target, "tenant", strconv.FormatInt(id, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminSetTenantAllowedSendersHandler — PUT /admin/tenants/{id}/allowed-senders
// Body: {"allowed_senders": ["Segtelco", "AcmeAlerts"]}
// Empty array (or omitted) clears the list → no restriction.
func AdminSetTenantAllowedSendersHandler(svc *tenancy.Service, audit *admin.Service) http.HandlerFunc {
	type req struct {
		AllowedSenders []string `json:"allowed_senders"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		if err := svc.SetAllowedSenders(r.Context(), id, in.AllowedSenders); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"tenant.allowed_senders.set", "tenant", strconv.FormatInt(id, 10),
			map[string]any{"senders": in.AllowedSenders})
		w.WriteHeader(http.StatusNoContent)
	}
}

// urlIDOrZero is a helper to read {id} as a chi url param without strconv noise.
// Currently inline in handlers; kept for future refactor.
var _ = chi.URLParam
