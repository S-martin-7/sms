package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
)

// AdminListMessagesHandler — GET /admin/messages with the same filters as
// /v1/sms plus optional ?tenant_id= for cross-tenant search.
func AdminListMessagesHandler(svc *sms.Service) http.HandlerFunc {
	type resp struct {
		Messages   []messageResp `json:"messages"`
		NextCursor *string       `json:"next_cursor"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		opts := sms.AdminListOpts{
			ListOpts: sms.ListOpts{
				Status:    strings.TrimSpace(q.Get("status")),
				Recipient: strings.TrimSpace(q.Get("recipient")),
				ClientRef: strings.TrimSpace(q.Get("client_ref")),
			},
		}
		if v := q.Get("tenant_id"); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant_id")
				return
			}
			opts.TenantID = id
		}
		if opts.Status != "" {
			if _, ok := validSMSStatuses[opts.Status]; !ok {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid status")
				return
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
			if err != nil || n < 1 || n > sms.MaxListLimit {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request",
					fmt.Sprintf("limit must be 1-%d", sms.MaxListLimit))
				return
			}
			opts.Limit = n
		}
		if v := q.Get("cursor"); v != "" {
			ts, id, err := httpx.DecodeMsgCursor(v)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
				return
			}
			opts.CursorCreatedAt = ts
			opts.CursorID = id
		}

		msgs, err := svc.AdminListMessages(r.Context(), opts)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "list failed")
			return
		}
		out := resp{Messages: make([]messageResp, 0, len(msgs))}
		for _, m := range msgs {
			out.Messages = append(out.Messages, toResp(m))
		}
		effectiveLimit := opts.Limit
		if effectiveLimit == 0 {
			effectiveLimit = sms.DefaultListLimit
		}
		if len(msgs) == effectiveLimit {
			last := msgs[len(msgs)-1]
			c := httpx.EncodeMsgCursor(last.CreatedAt, last.ID)
			out.NextCursor = &c
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}
