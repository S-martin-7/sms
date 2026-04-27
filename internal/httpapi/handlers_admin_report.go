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

type reportBucket struct {
	TS        time.Time `json:"ts"`
	Total     int64     `json:"total"`
	Delivered int64     `json:"delivered"`
	Failed    int64     `json:"failed"`
}

type reportTopRecipient struct {
	Recipient string `json:"recipient"`
	Total     int64  `json:"total"`
	Delivered int64  `json:"delivered"`
}

type reportTotals struct {
	Total        int64   `json:"total"`
	Delivered    int64   `json:"delivered"`
	Failed       int64   `json:"failed"`
	DeliveryRate float64 `json:"delivery_rate"`
}

type reportResp struct {
	From         time.Time            `json:"from"`
	To           time.Time            `json:"to"`
	Bucket       string               `json:"bucket"`         // "hour" | "day"
	Totals       reportTotals         `json:"totals"`
	Previous     *reportTotals        `json:"previous,omitempty"` // for delta vs prev period
	Series       []reportBucket       `json:"series"`
	TopRecipients []reportTopRecipient `json:"top_recipients"`
}

// AdminTenantReportHandler — GET /admin/tenants/{id}/report?from=&to=
//
// Devuelve métricas agregadas + serie temporal del cliente entre [from, to).
// Si el rango es <= 48h usamos buckets por hora, sino por día. Calcula también
// el período inmediatamente anterior (mismo largo) para que el frontend pueda
// mostrar deltas tipo "+12% vs período anterior".
func AdminTenantReportHandler(pool *pgxpool.Pool) http.HandlerFunc {
	q := sqlcgen.New(pool)
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		query := r.URL.Query()

		// Defaults: últimas 24h. `hours` es shortcut amigable; `from`/`to` lo
		// pisan para rangos custom.
		now := time.Now().UTC()
		from := now.Add(-24 * time.Hour)
		to := now
		if v := query.Get("hours"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 720*4 {
				from = now.Add(-time.Duration(n) * time.Hour)
			}
		}
		if v := query.Get("from"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "from must be RFC3339")
				return
			}
			from = t
		}
		if v := query.Get("to"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "to must be RFC3339")
				return
			}
			to = t
		}
		if !from.Before(to) {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "from must be < to")
			return
		}

		span := to.Sub(from)
		bucket := "day"
		if span <= 48*time.Hour {
			bucket = "hour"
		}

		// Time series del periodo actual.
		curr, err := q.AdminMessagesTimeBucketed(r.Context(), sqlcgen.AdminMessagesTimeBucketedParams{
			Bucket:   bucket,
			TenantID: &tenantID,
			FromTime: pgtype.Timestamptz{Time: from, Valid: true},
			ToTime:   pgtype.Timestamptz{Time: to, Valid: true},
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

		// Mismo largo, periodo anterior — para el delta.
		prevFrom := from.Add(-span)
		prevRows, err := q.AdminMessagesTimeBucketed(r.Context(), sqlcgen.AdminMessagesTimeBucketedParams{
			Bucket:   bucket,
			TenantID: &tenantID,
			FromTime: pgtype.Timestamptz{Time: prevFrom, Valid: true},
			ToTime:   pgtype.Timestamptz{Time: from, Valid: true},
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

		top, err := q.AdminTopRecipients(r.Context(), sqlcgen.AdminTopRecipientsParams{
			TenantID: tenantID,
			FromTime: pgtype.Timestamptz{Time: from, Valid: true},
			ToTime:   pgtype.Timestamptz{Time: to, Valid: true},
			Lim:      10,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

		out := reportResp{
			From:          from,
			To:            to,
			Bucket:        bucket,
			Series:        make([]reportBucket, 0, len(curr)),
			TopRecipients: make([]reportTopRecipient, 0, len(top)),
		}
		for _, r := range curr {
			out.Totals.Total += r.Total
			out.Totals.Delivered += r.Delivered
			out.Totals.Failed += r.Failed
			out.Series = append(out.Series, reportBucket{
				TS: r.Ts.Time, Total: r.Total, Delivered: r.Delivered, Failed: r.Failed,
			})
		}
		final := out.Totals.Delivered + out.Totals.Failed
		if final > 0 {
			out.Totals.DeliveryRate = float64(out.Totals.Delivered) / float64(final)
		}
		var prev reportTotals
		for _, r := range prevRows {
			prev.Total += r.Total
			prev.Delivered += r.Delivered
			prev.Failed += r.Failed
		}
		if prev.Delivered+prev.Failed > 0 {
			prev.DeliveryRate = float64(prev.Delivered) / float64(prev.Delivered+prev.Failed)
		}
		out.Previous = &prev

		for _, t := range top {
			out.TopRecipients = append(out.TopRecipients, reportTopRecipient{
				Recipient: t.Recipient, Total: t.Total, Delivered: t.Delivered,
			})
		}

		httpx.WriteJSON(w, http.StatusOK, out)
	}
}
