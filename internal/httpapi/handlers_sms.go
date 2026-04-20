package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
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

// SendSMSHandler handles POST /v1/sms — enqueue a single message.
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
