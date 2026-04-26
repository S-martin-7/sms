package httpapi

import (
	"net/http"
	"strconv"
	"time"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type adminStatsTotals struct {
	Total        int64   `json:"total"`
	Queued       int64   `json:"queued"`
	Sent         int64   `json:"sent"`
	Delivered    int64   `json:"delivered"`
	Undelivered  int64   `json:"undelivered"`
	Rejected     int64   `json:"rejected"`
	DeliveryRate float64 `json:"delivery_rate"` // delivered / (total - queued - sent)
}

type adminStatsByTenant struct {
	TenantID  int64  `json:"tenant_id"`
	Name      string `json:"name"`
	Total     int64  `json:"total"`
	Delivered int64  `json:"delivered"`
	Rejected  int64  `json:"rejected"`
}

type adminStatsRecentFailure struct {
	ID           string    `json:"id"`
	TenantID     int64     `json:"tenant_id"`
	Recipient    string    `json:"recipient"`
	Status       string    `json:"status"`
	ErrorCode    *string   `json:"error_code,omitempty"`
	ErrorMessage *string   `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type adminStatsStuckDelivery struct {
	ID         int64     `json:"id"`
	TenantID   int64     `json:"tenant_id"`
	EndpointID int64     `json:"endpoint_id"`
	EventType  string    `json:"event_type"`
	Status     string    `json:"status"`
	Attempts   int32     `json:"attempts"`
	LastStatus *int32    `json:"last_status,omitempty"`
	LastError  *string   `json:"last_error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type adminStatsResp struct {
	WindowHours      int                       `json:"window_hours"`
	Totals           adminStatsTotals          `json:"totals"`
	TopTenants       []adminStatsByTenant      `json:"top_tenants"`
	RecentFailures   []adminStatsRecentFailure `json:"recent_failures"`
	StuckDeliveries  []adminStatsStuckDelivery `json:"stuck_deliveries"`
}

// AdminStatsHandler — GET /admin/stats?hours=24
//
// Aggregates message + delivery stats over the last N hours (default 24).
// Cheap query: a few index scans on partial indexes already in place.
func AdminStatsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	q := sqlcgen.New(pool)
	return func(w http.ResponseWriter, r *http.Request) {
		hours := 24
		if v := r.URL.Query().Get("hours"); v != "" {
			n, err := strconv.Atoi(v)
			if err == nil && n > 0 && n <= 720 { // cap at 30 days
				hours = n
			}
		}
		cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
		cutoffPg := pgtype.Timestamptz{Time: cutoff, Valid: true}

		totalsRow, err := q.AdminStatsTotals(r.Context(), cutoffPg)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		topRows, err := q.AdminStatsByTenant(r.Context(), sqlcgen.AdminStatsByTenantParams{
			CreatedAt: cutoffPg, Limit: 5,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		failRows, err := q.AdminStatsRecentFailures(r.Context(), sqlcgen.AdminStatsRecentFailuresParams{
			CreatedAt: cutoffPg, Limit: 10,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		stuckRows, err := q.AdminStatsStuckDeliveries(r.Context(), sqlcgen.AdminStatsStuckDeliveriesParams{
			CreatedAt: cutoffPg, Limit: 10,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

		// Delivery rate considers only messages that had a chance to receive
		// a final status — exclude those still in-flight (queued/sending/sent).
		final := totalsRow.Delivered + totalsRow.Undelivered + totalsRow.Rejected
		var rate float64
		if final > 0 {
			rate = float64(totalsRow.Delivered) / float64(final)
		}

		out := adminStatsResp{
			WindowHours: hours,
			Totals: adminStatsTotals{
				Total:        totalsRow.Total,
				Queued:       totalsRow.Queued,
				Sent:         totalsRow.Sent,
				Delivered:    totalsRow.Delivered,
				Undelivered:  totalsRow.Undelivered,
				Rejected:     totalsRow.Rejected,
				DeliveryRate: rate,
			},
			TopTenants:      make([]adminStatsByTenant, 0, len(topRows)),
			RecentFailures:  make([]adminStatsRecentFailure, 0, len(failRows)),
			StuckDeliveries: make([]adminStatsStuckDelivery, 0, len(stuckRows)),
		}
		for _, r := range topRows {
			out.TopTenants = append(out.TopTenants, adminStatsByTenant{
				TenantID: r.ID, Name: r.Name, Total: r.Total,
				Delivered: r.Delivered, Rejected: r.Rejected,
			})
		}
		for _, r := range failRows {
			out.RecentFailures = append(out.RecentFailures, adminStatsRecentFailure{
				ID:           uuidString(r.ID),
				TenantID:     r.TenantID,
				Recipient:    r.Recipient,
				Status:       r.Status,
				ErrorCode:    r.ErrorCode,
				ErrorMessage: r.ErrorMessage,
				CreatedAt:    r.CreatedAt.Time,
			})
		}
		for _, r := range stuckRows {
			out.StuckDeliveries = append(out.StuckDeliveries, adminStatsStuckDelivery{
				ID:         r.ID,
				TenantID:   r.TenantID,
				EndpointID: r.EndpointID,
				EventType:  r.EventType,
				Status:     r.Status,
				Attempts:   r.Attempts,
				LastStatus: r.LastStatus,
				LastError:  r.LastError,
				CreatedAt:  r.CreatedAt.Time,
			})
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// uuidString formats a pgtype.UUID as the standard 8-4-4-4-12 hex form.
// (Repeated from internal/webhooks for now — small enough not to extract.)
func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	pos := 0
	for i, by := range u.Bytes {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[pos] = '-'
			pos++
		}
		out[pos] = hex[by>>4]
		out[pos+1] = hex[by&0x0f]
		pos += 2
	}
	return string(out)
}
