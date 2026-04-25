package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/webhooks"
)

// AdminListEndpointsForTenantHandler — GET /admin/tenants/{id}/webhooks
// Same shape as the tenant-facing list, just behind admin auth so the
// dashboard can show what's configured per tenant.
func AdminListEndpointsForTenantHandler(svc *webhooks.Service) http.HandlerFunc {
	type resp struct {
		Endpoints []webhookEndpointResp `json:"endpoints"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		eps, err := svc.ListEndpoints(r.Context(), tenantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		out := resp{Endpoints: make([]webhookEndpointResp, 0, len(eps))}
		for _, e := range eps {
			out.Endpoints = append(out.Endpoints, toEndpointResp(e))
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

type adminDeliveryResp struct {
	ID            int64      `json:"id"`
	EndpointID    int64      `json:"endpoint_id"`
	TenantID      int64      `json:"tenant_id"`
	EventID       string     `json:"event_id"`
	EventType     string     `json:"event_type"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LastStatus    *int32     `json:"last_status,omitempty"`
	LastError     *string    `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	DeliveredAt   *time.Time `json:"delivered_at,omitempty"`
}

// AdminListDeliveriesHandler — GET /admin/tenants/{id}/webhook-deliveries
//   ?status=pending|in_flight|success|failed|dead
//   ?cursor= int cursor (BIGSERIAL id)
//   ?limit= 1-200
func AdminListDeliveriesHandler(svc *webhooks.Service) http.HandlerFunc {
	type resp struct {
		Deliveries []adminDeliveryResp `json:"deliveries"`
		NextCursor *string             `json:"next_cursor"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		q := r.URL.Query()
		opts := webhooks.ListDeliveriesOpts{
			Status: strings.TrimSpace(q.Get("status")),
		}
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 200 {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "limit must be 1-200")
				return
			}
			opts.Limit = n
		}
		if v := q.Get("cursor"); v != "" {
			id, err := httpx.DecodeIntCursor(v)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
				return
			}
			opts.CursorID = id
		}

		rows, err := svc.ListDeliveriesByTenant(r.Context(), tenantID, opts)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		out := resp{Deliveries: make([]adminDeliveryResp, 0, len(rows))}
		for _, d := range rows {
			out.Deliveries = append(out.Deliveries, adminDeliveryResp{
				ID:            d.ID,
				EndpointID:    d.EndpointID,
				TenantID:      d.TenantID,
				EventID:       d.EventID.String(),
				EventType:     d.EventType,
				Status:        d.Status,
				Attempts:      d.Attempts,
				NextAttemptAt: d.NextAttemptAt,
				LastStatus:    d.LastStatus,
				LastError:     d.LastError,
				CreatedAt:     d.CreatedAt,
				DeliveredAt:   d.DeliveredAt,
			})
		}
		effectiveLimit := opts.Limit
		if effectiveLimit == 0 {
			effectiveLimit = 50
		}
		if len(rows) == effectiveLimit {
			c := httpx.EncodeIntCursor(rows[len(rows)-1].ID)
			out.NextCursor = &c
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// AdminRetryDeliveryHandler — POST /admin/webhook-deliveries/{id}/retry
// Resets the delivery to pending so the dispatcher will pick it up on the
// next poll. Useful for replaying after a tenant fixes their endpoint.
func AdminRetryDeliveryHandler(svc *webhooks.Service, audit *admin.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid delivery id")
			return
		}
		if err := svc.RequeueDelivery(r.Context(), id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"webhook_delivery.retry", "webhook_delivery", strconv.FormatInt(id, 10), nil)
		httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "requeued"})
	}
}

