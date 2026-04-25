package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/webhooks"
	"github.com/rs/zerolog"
)

// Maximum body size we'll accept from a Horisen callback. DLR/MO payloads
// are tiny JSON; anything larger is almost certainly an attack.
const horisenCallbackBodyLimit = 64 * 1024 // 64 KiB

// DLRHandler parses a Horisen DLR callback, applies the status update, and
// fans the event out to subscribed webhook endpoints. Always returns 200
// once the body is read — Horisen does not retry on 4xx, so dropping a
// malformed DLR is preferable to losing all subsequent ones.
func DLRHandler(svc *sms.Service, whSvc *webhooks.Service, log *zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := readCallbackBody(w, r, log, "dlr")
		if !ok {
			return
		}

		dlr, err := sms.ParseDLR(body)
		if err != nil {
			log.Warn().Err(err).
				Str("kind", "dlr").
				Str("raw", truncate(string(body), 512)).
				Msg("dlr parse failed")
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "parse_error"})
			return
		}

		res, err := svc.ApplyDLR(r.Context(), dlr)
		if err != nil {
			if errors.Is(err, sms.ErrDLRMessageNotFound) {
				log.Warn().
					Str("kind", "dlr").
					Str("custom_msg_id", dlr.Custom.MsgID).
					Str("horisen_msg_id", dlr.HorisenMsgID).
					Str("event", dlr.Event).
					Msg("dlr for unknown message — ignored")
				httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "unknown_message"})
				return
			}
			log.Error().Err(err).Str("kind", "dlr").Msg("dlr apply failed")
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "error"})
			return
		}

		evt := log.Info().
			Str("kind", "dlr").
			Str("event", dlr.Event).
			Str("msg_id", res.MsgID.String()).
			Int64("tenant_id", res.TenantID)
		if res.Skipped {
			evt.Str("skip_reason", res.SkipReason).Msg("dlr skipped")
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		evt.Str("new_status", res.NewStatus).Msg("dlr applied")

		// Fan out to webhook subscribers. Failures here are non-fatal for
		// the DLR ack — the row is already updated.
		if whSvc != nil {
			eventType := webhookEventForStatus(res.NewStatus)
			if eventType != "" {
				payload := webhookSMSEvent{
					Type:        eventType,
					MessageID:   res.MsgID.String(),
					TenantID:    res.TenantID,
					Status:      res.NewStatus,
					HorisenMsgID: dlr.HorisenMsgID,
					ErrorCode:   parseErrorCode(dlr.ErrorCode),
					ErrorMessage: dlrErrorMessage(dlr.ErrorMessage),
					NumParts:    dlr.NumParts,
					PartNum:     dlr.PartNum,
					Timestamp:   time.Now().UTC(),
				}
				count, ferr := whSvc.FanOut(r.Context(), res.TenantID, eventType, payload)
				if ferr != nil {
					log.Warn().Err(ferr).Str("event_type", eventType).Msg("webhook fanout failed")
				} else if count > 0 {
					log.Info().
						Str("event_type", eventType).
						Int("deliveries", count).
						Str("msg_id", res.MsgID.String()).
						Msg("webhook fanout enqueued")
				}
			}
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// webhookSMSEvent is the payload shape tenants receive on their endpoints.
// Stable: changing field names breaks downstream consumers.
type webhookSMSEvent struct {
	Type         string    `json:"type"`
	MessageID    string    `json:"message_id"`
	TenantID     int64     `json:"tenant_id"`
	Status       string    `json:"status"`
	HorisenMsgID string    `json:"horisen_msg_id,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	NumParts     int       `json:"num_parts,omitempty"`
	PartNum      int       `json:"part_num,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

func webhookEventForStatus(status string) string {
	switch status {
	case "delivered":
		return webhooks.EventSMSDelivered
	case "undelivered":
		return webhooks.EventSMSUndelivered
	case "rejected":
		return webhooks.EventSMSRejected
	default:
		return ""
	}
}

func parseErrorCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "0" {
			return ""
		}
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		if n == "0" {
			return ""
		}
		return n.String()
	}
	return ""
}

func dlrErrorMessage(s string) string {
	if s == "No error" {
		return ""
	}
	return s
}

// MOStubHandler accepts Horisen MO (inbound SMS) callbacks, logs, and returns 200.
// Will be replaced by a real handler once inbound_numbers routing exists.
func MOStubHandler(log *zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := readCallbackBody(w, r, log, "mo")
		if !ok {
			return
		}
		var parsed any
		evt := log.Info().Str("kind", "mo").Int("size", len(body))
		if json.Unmarshal(body, &parsed) == nil {
			evt.Interface("payload", parsed).Msg("horisen callback received (stub)")
		} else {
			evt.Str("raw", truncate(string(body), 512)).Msg("horisen callback received (non-json, stub)")
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func readCallbackBody(w http.ResponseWriter, r *http.Request, log *zerolog.Logger, kind string) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, horisenCallbackBodyLimit+1))
	if err != nil {
		log.Warn().Err(err).Str("kind", kind).Msg("horisen callback: body read failed")
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "could not read body")
		return nil, false
	}
	if len(body) > horisenCallbackBodyLimit {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "too_large", "body exceeds 64KiB")
		return nil, false
	}
	return body, true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
