package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/rs/zerolog"
)

// Maximum body size we'll accept from a Horisen callback. DLR/MO payloads
// are tiny JSON; anything larger is almost certainly an attack.
const horisenCallbackBodyLimit = 64 * 1024 // 64 KiB

// DLRHandler parses a Horisen DLR callback and applies the status update.
// Always returns 200 once the body is read — Horisen does not retry on 4xx,
// so dropping a malformed DLR is preferable to losing all subsequent ones.
func DLRHandler(svc *sms.Service, log *zerolog.Logger) http.HandlerFunc {
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
			// Still 200 — see comment above.
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
		} else {
			evt.Str("new_status", res.NewStatus).Msg("dlr applied")
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
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
