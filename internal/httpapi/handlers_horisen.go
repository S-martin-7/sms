package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/rs/zerolog"
)

// Maximum body size we'll accept from a Horisen callback. DLR/MO payloads
// are tiny JSON; anything larger is almost certainly an attack.
const horisenCallbackBodyLimit = 64 * 1024 // 64 KiB

// DLRStubHandler accepts Horisen DLR callbacks, logs the body, and returns 200.
//
// Stub: this does not yet parse or persist the DLR. It exists so we can register
// the URL with Horisen and confirm wire-level connectivity before implementing
// the parser + database update path.
func DLRStubHandler(log *zerolog.Logger) http.HandlerFunc {
	return horisenStub(log, "dlr")
}

// MOStubHandler accepts Horisen MO (inbound SMS) callbacks, logs, and returns 200.
func MOStubHandler(log *zerolog.Logger) http.HandlerFunc {
	return horisenStub(log, "mo")
}

func horisenStub(log *zerolog.Logger, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, horisenCallbackBodyLimit+1))
		if err != nil {
			log.Warn().Err(err).Str("kind", kind).Msg("horisen callback: body read failed")
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "could not read body")
			return
		}
		if len(body) > horisenCallbackBodyLimit {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "too_large", "body exceeds 64KiB")
			return
		}

		evt := log.Info().
			Str("kind", kind).
			Str("content_type", r.Header.Get("Content-Type")).
			Int("size", len(body))

		// Try to log the parsed JSON; fall back to raw string preview if not JSON.
		var parsed any
		if json.Unmarshal(body, &parsed) == nil {
			evt.Interface("payload", parsed).Msg("horisen callback received")
		} else {
			preview := string(body)
			if len(preview) > 512 {
				preview = preview[:512] + "...(truncated)"
			}
			evt.Str("raw", preview).Msg("horisen callback received (non-json)")
		}

		// Respond fast — Horisen retries on non-2xx.
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
