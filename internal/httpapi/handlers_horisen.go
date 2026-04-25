package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/S-martin-7/sms/internal/events"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/S-martin-7/sms/internal/webhooks"
	"github.com/rs/zerolog"
)

// Maximum body size we'll accept from a Horisen callback. DLR/MO payloads
// are tiny JSON; anything larger is almost certainly an attack.
const horisenCallbackBodyLimit = 64 * 1024 // 64 KiB

// DLRHandler parses a Horisen DLR callback, applies the status update,
// persists the event to the polling feed, and fans out a signed event to
// subscribed webhook endpoints. Always returns 200 once the body is read —
// Horisen does not retry on 4xx, so dropping a malformed DLR is preferable
// to losing all subsequent ones.
func DLRHandler(svc *sms.Service, whSvc *webhooks.Service, evSvc *events.Service, log *zerolog.Logger) http.HandlerFunc {
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

		eventType := webhookEventForStatus(res.NewStatus)
		if eventType == "" {
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
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

		// Persist to polling feed first — even if no webhooks are
		// subscribed (or fan-out fails) the tenant can backfill via API.
		if evSvc != nil {
			if _, err := evSvc.Create(r.Context(), res.TenantID, eventType, payload); err != nil {
				log.Warn().Err(err).Str("event_type", eventType).Msg("event persist failed")
			}
		}

		// Fan out to webhook subscribers. Failures here are non-fatal for
		// the DLR ack — the row is already updated and the event recorded.
		if whSvc != nil {
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

// MOHandler parses a Horisen MO callback, persists the inbound message
// scoped to the tenant that owns the destination MSISDN, records the event
// to the polling feed, and fans out a signed `sms.inbound` event to
// subscribed webhook endpoints.
//
// Always returns 200 once the body is read (Horisen does not retry on 4xx).
// MOs to unknown destination numbers are logged and dropped.
func MOHandler(svc *sms.Service, whSvc *webhooks.Service, evSvc *events.Service, log *zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := readCallbackBody(w, r, log, "mo")
		if !ok {
			return
		}

		mo, err := sms.ParseMO(body)
		if err != nil {
			log.Warn().Err(err).
				Str("kind", "mo").
				Str("raw", truncate(string(body), 512)).
				Msg("mo parse failed")
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "parse_error"})
			return
		}

		res, err := svc.ApplyMO(r.Context(), mo)
		if err != nil {
			log.Error().Err(err).Str("kind", "mo").Msg("mo apply failed")
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "error"})
			return
		}
		if res.Skipped {
			log.Warn().
				Str("kind", "mo").
				Str("dst", mo.Dest).
				Str("src", mo.Source).
				Str("reason", res.SkipReason).
				Msg("mo dropped — no tenant route")
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "no_route"})
			return
		}

		log.Info().
			Str("kind", "mo").
			Str("inbound_id", res.Inbound.ID.String()).
			Int64("tenant_id", res.Inbound.TenantID).
			Str("src", res.Inbound.Src).
			Str("dst", res.Inbound.Dst).
			Msg("mo persisted")

		payload := webhookMOEvent{
			Type:       webhooks.EventSMSInbound,
			MessageID:  res.Inbound.ID.String(),
			TenantID:   res.Inbound.TenantID,
			Src:        res.Inbound.Src,
			Dst:        res.Inbound.Dst,
			Text:       res.Inbound.Text,
			DCS:        res.Inbound.DCS,
			ReceivedAt: res.Inbound.ReceivedAt.UTC(),
		}

		// Persist to polling feed.
		if evSvc != nil {
			if _, err := evSvc.Create(r.Context(), res.Inbound.TenantID, webhooks.EventSMSInbound, payload); err != nil {
				log.Warn().Err(err).Msg("mo event persist failed")
			}
		}

		// Fan out webhook event.
		if whSvc != nil {
			count, ferr := whSvc.FanOut(r.Context(), res.Inbound.TenantID, webhooks.EventSMSInbound, payload)
			if ferr != nil {
				log.Warn().Err(ferr).Msg("mo webhook fanout failed")
			} else if count > 0 {
				log.Info().
					Int("deliveries", count).
					Str("inbound_id", res.Inbound.ID.String()).
					Msg("mo webhook fanout enqueued")
			}
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// webhookMOEvent is the payload shape tenants receive on `sms.inbound`.
type webhookMOEvent struct {
	Type       string    `json:"type"`
	MessageID  string    `json:"message_id"`
	TenantID   int64     `json:"tenant_id"`
	Src        string    `json:"src"`
	Dst        string    `json:"dst"`
	Text       string    `json:"text"`
	DCS        string    `json:"dcs,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
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
