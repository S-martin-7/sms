package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type sendReq struct {
	Sender    string `json:"sender"`
	To        string `json:"to"`
	Text      string `json:"text"`
	ClientRef string `json:"client_ref,omitempty"`
	// Optional: when set, the message is queued in scheduled_sends instead of
	// being dispatched immediately. RFC3339 timestamp; the scheduler picks it
	// up at-or-after this time and runs through the same outbox path.
	SendAt string `json:"send_at,omitempty"`
}

type scheduledResp struct {
	ScheduledID int64     `json:"scheduled_id"`
	TenantID    int64     `json:"tenant_id"`
	Sender      string    `json:"sender"`
	To          string    `json:"to,omitempty"`
	Recipients  []string  `json:"recipients,omitempty"`
	Text        string    `json:"text"`
	Status      string    `json:"status"` // "scheduled"
	SendAt      time.Time `json:"send_at"`
}

type messageResp struct {
	ID           string     `json:"id"`
	TenantID     int64      `json:"tenant_id"`
	Sender       string     `json:"sender"`
	Recipient    string     `json:"to"`
	Text         string     `json:"text"`
	DCS          string     `json:"dcs"`
	NumParts     int        `json:"num_parts"`
	Status       string     `json:"status"`
	HorisenMsgID *string    `json:"horisen_msg_id,omitempty"`
	ErrorCode    *string    `json:"error_code,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	ClientRef    *string    `json:"client_ref,omitempty"`
	Attempts     int        `json:"attempts"`
	CreatedAt    time.Time  `json:"created_at"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
	FinalAt      *time.Time `json:"final_at,omitempty"`
}

func toResp(m *sms.Message) messageResp {
	return messageResp{
		ID:           m.ID.String(),
		TenantID:     m.TenantID,
		Sender:       m.Sender,
		Recipient:    m.Recipient,
		Text:         m.Text,
		DCS:          string(m.DCS),
		NumParts:     m.NumParts,
		Status:       m.Status,
		HorisenMsgID: m.HorisenMsgID,
		ErrorCode:    m.ErrorCode,
		ErrorMessage: m.ErrorMessage,
		ClientRef:    m.ClientRef,
		Attempts:     m.Attempts,
		CreatedAt:    m.CreatedAt,
		SentAt:       m.SentAt,
		FinalAt:      m.FinalAt,
	}
}

// MaxBulkBatchSize caps a single POST /v1/sms/bulk to keep memory + DB
// load predictable. Tenants with bigger sends should chunk client-side.
const MaxBulkBatchSize = 1000

type bulkSendReq struct {
	DefaultSender string         `json:"default_sender"`
	Messages      []bulkSendItem `json:"messages"`
	// Optional: when set, the WHOLE batch is scheduled instead of dispatched
	// immediately. Per-row send_at is not supported (use multiple POSTs for
	// staggered scheduling).
	SendAt string `json:"send_at,omitempty"`
}

type bulkSendItem struct {
	To        string `json:"to"`
	Text      string `json:"text"`
	Sender    string `json:"sender,omitempty"`     // overrides default_sender for this row
	ClientRef string `json:"client_ref,omitempty"` // optional idempotency key
}

type bulkResultRow struct {
	Index     int     `json:"index"`               // matches request position
	ID        string  `json:"id,omitempty"`        // populated on accepted
	To        string  `json:"to"`
	Status    string  `json:"status"`              // queued|rejected
	ClientRef *string `json:"client_ref,omitempty"`
	Error     string  `json:"error,omitempty"`     // populated on rejected
	ErrorCode string  `json:"error_code,omitempty"`
}

type bulkSendResp struct {
	BatchID  string          `json:"batch_id"`
	Accepted int             `json:"accepted"`
	Rejected int             `json:"rejected"`
	Messages []bulkResultRow `json:"messages"`
}

// SendBulkSMSHandler handles POST /v1/sms/bulk — enqueue many messages in
// one request. Per-row partial accept: validation/duplicate failures are
// reported in the response without blocking the rest.
func SendBulkSMSHandler(svc *sms.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		if tenantID == 0 {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing tenant")
			return
		}

		var in bulkSendReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		if len(in.Messages) == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "messages must be non-empty")
			return
		}
		if len(in.Messages) > MaxBulkBatchSize {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request",
				fmt.Sprintf("batch too large: %d > %d", len(in.Messages), MaxBulkBatchSize))
			return
		}
		defaultSender := strings.TrimSpace(in.DefaultSender)

		// Bulk scheduling: schedule the whole batch as one scheduled_send row
		// targeting all recipients. The scheduler dispatches them via
		// EnqueueBulk at the configured time.
		if in.SendAt != "" {
			when, err := time.Parse(time.RFC3339, in.SendAt)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "send_at must be RFC3339")
				return
			}
			if when.Before(time.Now().Add(-1 * time.Minute)) {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "send_at is in the past")
				return
			}
			recipients := make([]string, 0, len(in.Messages))
			text := ""
			for _, m := range in.Messages {
				to := strings.TrimSpace(m.To)
				if to == "" || m.Text == "" {
					httpx.WriteError(w, http.StatusBadRequest, "bad_request", "every message needs to+text")
					return
				}
				if text == "" {
					text = m.Text
				} else if text != m.Text {
					httpx.WriteError(w, http.StatusBadRequest, "bad_request",
						"scheduled bulk requires the same text for every recipient")
					return
				}
				recipients = append(recipients, to)
			}
			row, err := svc.EnqueueScheduledSend(r.Context(), tenantID,
				defaultSender, text, recipients, when, nil, "API bulk send")
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
				return
			}
			httpx.WriteJSON(w, http.StatusAccepted, scheduledResp{
				ScheduledID: row.ID, TenantID: row.TenantID,
				Sender: row.Sender, Recipients: recipients, Text: row.Text,
				Status: "scheduled", SendAt: row.SendAt.Time,
			})
			return
		}

		inputs := make([]sms.EnqueueInput, len(in.Messages))
		for i, m := range in.Messages {
			sender := strings.TrimSpace(m.Sender)
			if sender == "" {
				sender = defaultSender
			}
			inputs[i] = sms.EnqueueInput{
				TenantID:  tenantID,
				Sender:    sender,
				Recipient: strings.TrimSpace(m.To),
				Text:      m.Text,
				ClientRef: strings.TrimSpace(m.ClientRef),
			}
		}

		results := svc.EnqueueBulk(r.Context(), inputs)

		out := bulkSendResp{
			BatchID:  "b_" + uuid.New().String(),
			Messages: make([]bulkResultRow, len(results)),
		}
		for i, res := range results {
			row := bulkResultRow{
				Index: i,
				To:    in.Messages[i].To,
			}
			if ref := strings.TrimSpace(in.Messages[i].ClientRef); ref != "" {
				row.ClientRef = &ref
			}
			if res.Err != nil {
				row.Status = "rejected"
				row.Error = res.Err.Error()
				row.ErrorCode = bulkErrorCode(res.Err)
				out.Rejected++
			} else {
				row.Status = "queued"
				row.ID = res.Msg.ID.String()
				out.Accepted++
			}
			out.Messages[i] = row
		}
		httpx.WriteJSON(w, http.StatusAccepted, out)
	}
}

// bulkErrorCode maps known error sentinels to a stable, machine-friendly
// code. Anything unknown bucketed as bad_request.
func bulkErrorCode(err error) string {
	switch {
	case errors.Is(err, sms.ErrDuplicateClientRef):
		return "duplicate_client_ref"
	default:
		return "bad_request"
	}
}

// SendSMSHandler handles POST /v1/sms — enqueue a single message OR schedule
// it for future delivery if `send_at` is in the body.
func SendSMSHandler(svc *sms.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		if tenantID == 0 {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing tenant")
			return
		}

		var in sendReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		in.Sender = strings.TrimSpace(in.Sender)
		in.To = strings.TrimSpace(in.To)
		if in.Sender == "" || in.To == "" || in.Text == "" {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "sender, to and text required")
			return
		}

		// Scheduling path — same shape as immediate, plus send_at.
		if in.SendAt != "" {
			when, err := time.Parse(time.RFC3339, in.SendAt)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "send_at must be RFC3339")
				return
			}
			if when.Before(time.Now().Add(-1 * time.Minute)) {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "send_at is in the past")
				return
			}
			row, err := svc.EnqueueScheduledSend(r.Context(), tenantID,
				in.Sender, in.Text, []string{in.To}, when, nil, "API single send")
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
				return
			}
			httpx.WriteJSON(w, http.StatusAccepted, scheduledResp{
				ScheduledID: row.ID, TenantID: row.TenantID,
				Sender: row.Sender, To: in.To, Text: row.Text,
				Status: "scheduled", SendAt: row.SendAt.Time,
			})
			return
		}

		msg, err := svc.Enqueue(r.Context(), sms.EnqueueInput{
			TenantID:  tenantID,
			Sender:    in.Sender,
			Recipient: in.To,
			Text:      in.Text,
			ClientRef: in.ClientRef,
		})
		if err != nil {
			if errors.Is(err, sms.ErrDuplicateClientRef) {
				httpx.WriteError(w, http.StatusConflict, "duplicate_client_ref", "client_ref already used")
				return
			}
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, toResp(msg))
	}
}

// validSMSStatuses are the values we accept on `?status=` for /v1/sms.
// Anything else returns 400 instead of silently returning [].
var validSMSStatuses = map[string]struct{}{
	"queued":      {},
	"sending":     {},
	"sent":        {},
	"delivered":   {},
	"undelivered": {},
	"rejected":    {},
	"failed":      {},
}

// ListSMSHandler — GET /v1/sms?status=&recipient=&client_ref=&from=&to=&cursor=&limit=
func ListSMSHandler(svc *sms.Service) http.HandlerFunc {
	type resp struct {
		Messages   []messageResp `json:"messages"`
		NextCursor *string       `json:"next_cursor"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		q := r.URL.Query()

		opts := sms.ListOpts{
			Status:    strings.TrimSpace(q.Get("status")),
			Recipient: strings.TrimSpace(q.Get("recipient")),
			ClientRef: strings.TrimSpace(q.Get("client_ref")),
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

		msgs, err := svc.ListMessages(r.Context(), tenantID, opts)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "list failed")
			return
		}

		out := resp{Messages: make([]messageResp, 0, len(msgs))}
		for _, m := range msgs {
			out.Messages = append(out.Messages, toResp(m))
		}
		// Emit a next_cursor only when the page is full — saves the client
		// from making an extra empty request to discover end-of-list.
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

// GetSMSHandler handles GET /v1/sms/{id}.
func GetSMSHandler(svc *sms.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := httpx.TenantIDFrom(r.Context())
		raw := chi.URLParam(r, "id")
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		msg, err := svc.GetForTenant(r.Context(), id, tenantID)
		if errors.Is(err, sms.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "no such message")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toResp(msg))
	}
}
