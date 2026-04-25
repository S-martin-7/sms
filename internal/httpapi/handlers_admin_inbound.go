package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/go-chi/chi/v5"
)

type adminInboundNumberResp struct {
	MSISDN    string    `json:"msisdn"`
	TenantID  int64     `json:"tenant_id"`
	Label     string    `json:"label,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminListInboundNumbersHandler — GET /admin/inbound-numbers
func AdminListInboundNumbersHandler(svc *sms.Service) http.HandlerFunc {
	type resp struct {
		Numbers []adminInboundNumberResp `json:"numbers"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := svc.ListInboundNumbers(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "list failed")
			return
		}
		out := resp{Numbers: make([]adminInboundNumberResp, 0, len(rows))}
		for _, n := range rows {
			out.Numbers = append(out.Numbers, adminInboundNumberResp{
				MSISDN:    n.MSISDN,
				TenantID:  n.TenantID,
				Label:     n.Label,
				CreatedAt: n.CreatedAt,
			})
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// AdminAssignInboundNumberHandler — POST /admin/inbound-numbers
//   { "msisdn": "569...", "tenant_id": 1, "label": "..." }
func AdminAssignInboundNumberHandler(svc *sms.Service, audit *admin.Service) http.HandlerFunc {
	type req struct {
		MSISDN   string `json:"msisdn"`
		TenantID int64  `json:"tenant_id"`
		Label    string `json:"label,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		row, err := svc.AssignInboundNumber(r.Context(), in.MSISDN, in.TenantID, in.Label)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"inbound_number.assign", "inbound_number", row.MSISDN,
			map[string]any{"tenant_id": row.TenantID, "label": row.Label})
		httpx.WriteJSON(w, http.StatusCreated, adminInboundNumberResp{
			MSISDN:    row.MSISDN,
			TenantID:  row.TenantID,
			Label:     row.Label,
			CreatedAt: row.CreatedAt,
		})
	}
}

// AdminUnassignInboundNumberHandler — DELETE /admin/inbound-numbers/{msisdn}
func AdminUnassignInboundNumberHandler(svc *sms.Service, audit *admin.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		msisdn := chi.URLParam(r, "msisdn")
		if msisdn == "" {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "msisdn required")
			return
		}
		if err := svc.UnassignInboundNumber(r.Context(), msisdn); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"inbound_number.unassign", "inbound_number", msisdn, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}
