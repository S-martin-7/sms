package httpapi

import (
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/httpx"
)

type pingResp struct {
	OK       bool      `json:"ok"`
	TenantID int64     `json:"tenant_id"`
	At       time.Time `json:"at"`
}

// PingHandler returns the caller's tenant id — auth pipeline smoke test.
func PingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, pingResp{
			OK:       true,
			TenantID: httpx.TenantIDFrom(r.Context()),
			At:       time.Now().UTC(),
		})
	}
}
