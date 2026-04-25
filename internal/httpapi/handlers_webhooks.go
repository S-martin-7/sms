package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/webhooks"
	"github.com/go-chi/chi/v5"
)

type webhookEndpointResp struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	// Secret is only present in the response of POST /v1/webhooks (the
	// create call). Subsequent GETs omit it because the database stores
	// it once and never reveals it again.
	Secret string `json:"secret,omitempty"`
}

func toEndpointResp(e *webhooks.Endpoint) webhookEndpointResp {
	return webhookEndpointResp{
		ID:        e.ID,
		URL:       e.URL,
		Events:    e.Events,
		Active:    e.Active,
		CreatedAt: e.CreatedAt,
		Secret:    e.Secret,
	}
}

// CreateWebhookHandler — POST /v1/webhooks
func CreateWebhookHandler(svc *webhooks.Service) http.HandlerFunc {
	type req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		ep, err := svc.CreateEndpoint(r.Context(), webhooks.CreateEndpointInput{
			TenantID: tenantID,
			URL:      in.URL,
			Events:   in.Events,
		})
		if err != nil {
			switch {
			case errors.Is(err, webhooks.ErrInvalidURL),
				errors.Is(err, webhooks.ErrInvalidEvent),
				errors.Is(err, webhooks.ErrNoEvents):
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			}
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, toEndpointResp(ep))
	}
}

// ListWebhooksHandler — GET /v1/webhooks
func ListWebhooksHandler(svc *webhooks.Service) http.HandlerFunc {
	type resp struct {
		Endpoints []webhookEndpointResp `json:"endpoints"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		eps, err := svc.ListEndpoints(r.Context(), tenantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		out := make([]webhookEndpointResp, 0, len(eps))
		for _, e := range eps {
			out = append(out, toEndpointResp(e))
		}
		httpx.WriteJSON(w, http.StatusOK, resp{Endpoints: out})
	}
}

// GetWebhookHandler — GET /v1/webhooks/{id}
func GetWebhookHandler(svc *webhooks.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		ep, err := svc.GetEndpoint(r.Context(), id, tenantID)
		if errors.Is(err, webhooks.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "no such endpoint")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toEndpointResp(ep))
	}
}

// DeleteWebhookHandler — DELETE /v1/webhooks/{id}
func DeleteWebhookHandler(svc *webhooks.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		if err := svc.DeleteEndpoint(r.Context(), id, tenantID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func parseInt64URLParam(r *http.Request, name string) (int64, bool) {
	raw := chi.URLParam(r, name)
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
