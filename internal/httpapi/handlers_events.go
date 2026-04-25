package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/S-martin-7/sms/internal/events"
	"github.com/S-martin-7/sms/internal/httpx"
)

type eventResp struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

// ListEventsHandler — GET /v1/events?types=sms.delivered,sms.inbound&from=&to=&cursor=&limit=
func ListEventsHandler(svc *events.Service) http.HandlerFunc {
	type resp struct {
		Events     []eventResp `json:"events"`
		NextCursor *string     `json:"next_cursor"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		q := r.URL.Query()

		opts := events.ListOpts{}
		if v := q.Get("types"); v != "" {
			parts := strings.Split(v, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					opts.Types = append(opts.Types, p)
				}
			}
		}
		if v := q.Get("from"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "from must be RFC3339")
				return
			}
			opts.From = t
		}
		if v := q.Get("to"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "to must be RFC3339")
				return
			}
			opts.To = t
		}
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > events.MaxLimit {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request",
					fmt.Sprintf("limit must be 1-%d", events.MaxLimit))
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

		rows, err := svc.List(r.Context(), tenantID, opts)
		if err != nil {
			if errors.Is(err, events.ErrInvalidType) {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "list failed")
			return
		}

		out := resp{Events: make([]eventResp, 0, len(rows))}
		for _, e := range rows {
			out.Events = append(out.Events, eventResp{
				ID:        e.ID,
				Type:      e.Type,
				CreatedAt: e.CreatedAt,
				Data:      e.Payload,
			})
		}
		effectiveLimit := opts.Limit
		if effectiveLimit == 0 {
			effectiveLimit = events.DefaultLimit
		}
		if len(rows) == effectiveLimit {
			c := httpx.EncodeIntCursor(rows[len(rows)-1].ID)
			out.NextCursor = &c
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}
