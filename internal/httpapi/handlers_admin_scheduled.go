package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/sms"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

type scheduledSendResp struct {
	ID             int64      `json:"id"`
	TenantID       int64      `json:"tenant_id"`
	Name           *string    `json:"name,omitempty"`
	Sender         string     `json:"sender"`
	Text           string     `json:"text"`
	Recipients     []string   `json:"recipients,omitempty"`
	ListID         *int64     `json:"list_id,omitempty"`
	RecipientCount int        `json:"recipient_count"`
	SendAt         time.Time  `json:"send_at"`
	Recurrence     *string    `json:"recurrence,omitempty"`
	RecurrenceDays []int      `json:"recurrence_days,omitempty"`
	Timezone       string     `json:"timezone"`
	Status         string     `json:"status"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastBatchID    *string    `json:"last_batch_id,omitempty"`
	TotalRuns      int        `json:"total_runs"`
	LastError      *string    `json:"last_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func toScheduledResp(r sqlcgen.ScheduledSend) scheduledSendResp {
	out := scheduledSendResp{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Sender:      r.Sender,
		Text:        r.Text,
		ListID:      r.ListID,
		SendAt:      r.SendAt.Time,
		Recurrence:  r.Recurrence,
		Timezone:    r.Timezone,
		Status:      r.Status,
		LastBatchID: r.LastBatchID,
		TotalRuns:   int(r.TotalRuns),
		LastError:   r.LastError,
		CreatedAt:   r.CreatedAt.Time,
	}
	if r.LastRunAt.Valid {
		t := r.LastRunAt.Time
		out.LastRunAt = &t
	}
	if len(r.RecurrenceDays) > 0 {
		days := make([]int, len(r.RecurrenceDays))
		for i, d := range r.RecurrenceDays {
			days[i] = int(d)
		}
		out.RecurrenceDays = days
	}
	if len(r.Recipients) > 0 {
		var rs []string
		if json.Unmarshal(r.Recipients, &rs) == nil {
			out.Recipients = rs
			out.RecipientCount = len(rs)
		}
	}
	return out
}

// AdminListScheduledHandler — GET /admin/tenants/{id}/scheduled
func AdminListScheduledHandler(pool *pgxpool.Pool) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type resp struct {
		Scheduled []scheduledSendResp `json:"scheduled"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		rows, err := q.ListScheduledSends(r.Context(), sqlcgen.ListScheduledSendsParams{
			TenantID: tenantID, Limit: 100,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		out := resp{Scheduled: make([]scheduledSendResp, 0, len(rows))}
		for _, r := range rows {
			out.Scheduled = append(out.Scheduled, toScheduledResp(r))
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// AdminCreateScheduledHandler — POST /admin/tenants/{id}/scheduled
//
// Body:
//   {
//     "name": "Recordatorio mensual",
//     "sender": "MiMarca",
//     "text": "Hola {nombre}",
//     "recipients": ["569...", "569..."],   // OR list_id
//     "list_id": 5,
//     "send_at": "2026-04-30T15:00:00-04:00",
//     "recurrence": "weekly",                // optional; null = one-shot
//     "recurrence_days": [1,3,5],            // 0=Sun..6=Sat (when weekly)
//     "timezone": "America/Santiago"
//   }
func AdminCreateScheduledHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type req struct {
		Name           string   `json:"name"`
		Sender         string   `json:"sender"`
		Text           string   `json:"text"`
		Recipients     []string `json:"recipients"`
		ListID         *int64   `json:"list_id"`
		SendAt         string   `json:"send_at"`
		Recurrence     string   `json:"recurrence"`
		RecurrenceDays []int    `json:"recurrence_days"`
		Timezone       string   `json:"timezone"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		row, err := createScheduledFromReq(r, q, tenantID, in.Name, in.Sender, in.Text,
			in.Recipients, in.ListID, in.SendAt, in.Recurrence, in.RecurrenceDays, in.Timezone)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"scheduled.create", "scheduled_send", strconv.FormatInt(row.ID, 10),
			map[string]any{"tenant_id": tenantID, "send_at": row.SendAt, "recipients": row.RecipientCount})
		httpx.WriteJSON(w, http.StatusCreated, *row)
	}
}

// shared logic used by both the JSON endpoint and the xlsx import.
func createScheduledFromReq(
	r *http.Request, q *sqlcgen.Queries,
	tenantID int64,
	name, sender, text string,
	recipients []string, listID *int64,
	sendAtRaw, recurrence string, days []int, tz string,
) (*scheduledSendResp, error) {
	sender = strings.TrimSpace(sender)
	text = strings.TrimSpace(text)
	if sender == "" || text == "" {
		return nil, fmt.Errorf("sender and text required")
	}
	hasList := listID != nil && *listID > 0
	hasRecipients := len(recipients) > 0
	if !hasList && !hasRecipients {
		return nil, fmt.Errorf("recipients[] or list_id required")
	}
	if sendAtRaw == "" {
		return nil, fmt.Errorf("send_at required (RFC3339)")
	}
	when, err := time.Parse(time.RFC3339, sendAtRaw)
	if err != nil {
		return nil, fmt.Errorf("send_at must be RFC3339")
	}
	// Allow up to 1 minute in the past for clock skew.
	if when.Before(time.Now().Add(-time.Minute)) {
		return nil, fmt.Errorf("send_at is in the past")
	}

	params := sqlcgen.CreateScheduledSendParams{
		TenantID: tenantID,
		Name:     ptrIfNotEmpty(name),
		Sender:   sender,
		Text:     text,
		SendAt:   pgtype.Timestamptz{Time: when, Valid: true},
		Column10: tz,
	}
	if hasList {
		params.ListID = listID
	} else {
		body, _ := json.Marshal(recipients)
		params.Recipients = body
	}
	if recurrence != "" {
		switch recurrence {
		case "weekly":
			if len(days) == 0 {
				return nil, fmt.Errorf("recurrence=weekly requires recurrence_days")
			}
			d := make([]int16, 0, len(days))
			for _, x := range days {
				if x < 0 || x > 6 {
					return nil, fmt.Errorf("recurrence_days values must be 0..6")
				}
				d = append(d, int16(x))
			}
			rec := "weekly"
			params.Recurrence = &rec
			params.RecurrenceDays = d
		default:
			return nil, fmt.Errorf("unknown recurrence: %s", recurrence)
		}
	}
	if id := httpx.AdminIDFrom(r.Context()); id != 0 {
		params.CreatedBy = &id
	}
	row, err := q.CreateScheduledSend(r.Context(), params)
	if err != nil {
		return nil, fmt.Errorf("insert scheduled: %w", err)
	}
	resp := toScheduledResp(row)
	return &resp, nil
}

// AdminPauseScheduledHandler — POST /admin/scheduled/{id}/pause
//   body { "paused": true|false }
func AdminPauseScheduledHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type req struct {
		Paused bool `json:"paused"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		var tenantID int64
		err := pool.QueryRow(r.Context(), `SELECT tenant_id FROM scheduled_sends WHERE id = $1`, id).Scan(&tenantID)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		newStatus := "pending"
		if in.Paused {
			newStatus = "paused"
		}
		if err := q.SetScheduledSendStatus(r.Context(), sqlcgen.SetScheduledSendStatusParams{
			ID: id, TenantID: tenantID, Status: newStatus,
		}); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"scheduled."+newStatus, "scheduled_send", strconv.FormatInt(id, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminDeleteScheduledHandler — DELETE /admin/scheduled/{id}
func AdminDeleteScheduledHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		var tenantID int64
		err := pool.QueryRow(r.Context(), `SELECT tenant_id FROM scheduled_sends WHERE id = $1`, id).Scan(&tenantID)
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if err := q.DeleteScheduledSend(r.Context(), sqlcgen.DeleteScheduledSendParams{ID: id, TenantID: tenantID}); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"scheduled.delete", "scheduled_send", strconv.FormatInt(id, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminImportScheduledXLSXHandler — POST /admin/tenants/{id}/scheduled/import
//
// Recibe un .xlsx (Content-Type opcional). Cada fila representa UN scheduled
// send. Columnas reconocidas (case-insensitive, header obligatorio):
//   msisdn / numero / phone   — destinatario único (también acepta varios separados por ; o ,)
//   sender / remitente        — opcional; usa default_sender si vacío
//   text / mensaje / texto    — texto del mensaje
//   send_at / fecha / fecha_envio  — timestamp ISO o "DD/MM/YYYY HH:MM"
//   recurrence / recurrencia  — opcional, "weekly" o vacío
//   days / dias               — opcional, dígitos 0..6 separados por coma (e.g. "1,3,5")
//
// Query: ?default_sender=MiMarca   se usa cuando una fila no trae sender.
//
// Devuelve { imported, skipped, errors[] } como el CSV de contactos.
func AdminImportScheduledXLSXHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type resp struct {
		Imported int      `json:"imported"`
		Skipped  int      `json:"skipped"`
		Errors   []string `json:"errors,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		defaultSender := strings.TrimSpace(r.URL.Query().Get("default_sender"))

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "read body: "+err.Error())
			return
		}
		f, err := excelize.OpenReader(strings.NewReader(string(body)))
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "parse xlsx: "+err.Error())
			return
		}
		defer f.Close()
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "xlsx has no sheets")
			return
		}
		rows, err := f.GetRows(sheets[0])
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "read sheet: "+err.Error())
			return
		}
		if len(rows) < 2 {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "xlsx requires at least 1 header row + 1 data row")
			return
		}

		// Header indexing.
		idx := map[string]int{}
		for i, h := range rows[0] {
			switch normalizeHeader(h) {
			case "msisdn", "numero", "number", "phone", "telefono":
				idx["msisdn"] = i
			case "sender", "remitente":
				idx["sender"] = i
			case "text", "mensaje", "texto":
				idx["text"] = i
			case "send_at", "fecha", "fecha_envio", "fecha envio":
				idx["send_at"] = i
			case "recurrence", "recurrencia":
				idx["recurrence"] = i
			case "days", "dias", "días":
				idx["days"] = i
			case "name", "nombre", "etiqueta":
				idx["name"] = i
			}
		}
		if _, ok := idx["msisdn"]; !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request",
				"falta columna msisdn (alias: numero, phone, telefono)")
			return
		}
		if _, ok := idx["text"]; !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request",
				"falta columna text (alias: mensaje, texto)")
			return
		}
		if _, ok := idx["send_at"]; !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request",
				"falta columna send_at (alias: fecha, fecha_envio)")
			return
		}

		out := resp{}
		for i := 1; i < len(rows); i++ {
			row := rows[i]
			cell := func(name string) string {
				if pos, ok := idx[name]; ok && pos < len(row) {
					return strings.TrimSpace(row[pos])
				}
				return ""
			}
			rawNums := cell("msisdn")
			text := cell("text")
			sendAt := cell("send_at")
			if rawNums == "" || text == "" || sendAt == "" {
				out.Skipped++
				continue
			}
			// Multiple destinos por fila si trae ; o ,
			parts := strings.FieldsFunc(rawNums, func(r rune) bool {
				return r == ',' || r == ';' || r == ' '
			})
			recipients := parts[:0]
			for _, p := range parts {
				p = strings.TrimSpace(strings.TrimPrefix(p, "+"))
				if p != "" {
					recipients = append(recipients, p)
				}
			}
			if len(recipients) == 0 {
				out.Skipped++
				continue
			}

			// send_at: try RFC3339 first, then DD/MM/YYYY HH:MM (CL local).
			when, perr := parseFlexibleDate(sendAt)
			if perr != nil {
				if len(out.Errors) < 25 {
					out.Errors = append(out.Errors, fmt.Sprintf("fila %d: send_at inválido (%s)", i+1, sendAt))
				}
				out.Skipped++
				continue
			}

			sender := cell("sender")
			if sender == "" {
				sender = defaultSender
			}
			if sender == "" {
				if len(out.Errors) < 25 {
					out.Errors = append(out.Errors, fmt.Sprintf("fila %d: sin sender (no se pasó default_sender ni columna sender)", i+1))
				}
				out.Skipped++
				continue
			}

			rec := cell("recurrence")
			var days []int
			if d := cell("days"); d != "" {
				for _, p := range strings.Split(d, ",") {
					n, err := strconv.Atoi(strings.TrimSpace(p))
					if err == nil && n >= 0 && n <= 6 {
						days = append(days, n)
					}
				}
			}

			_, cerr := createScheduledFromReq(
				r, q,
				tenantID,
				cell("name"),
				sender, text,
				recipients, nil,
				when.Format(time.RFC3339),
				rec, days,
				"America/Santiago",
			)
			if cerr != nil {
				if len(out.Errors) < 25 {
					out.Errors = append(out.Errors, fmt.Sprintf("fila %d: %v", i+1, cerr))
				}
				out.Skipped++
				continue
			}
			out.Imported++
		}

		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"scheduled.import_xlsx", "tenant", strconv.FormatInt(tenantID, 10),
			map[string]any{"imported": out.Imported, "skipped": out.Skipped})

		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// parseFlexibleDate tries RFC3339, then "DD/MM/YYYY HH:MM" (Chile local),
// then plain Excel date serial number (already converted to text by excelize).
func parseFlexibleDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	loc, _ := time.LoadLocation("America/Santiago")
	candidates := []string{
		"02/01/2006 15:04",
		"02-01-2006 15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"02/01/2006",
	}
	for _, layout := range candidates {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("formato de fecha no reconocido")
}

// avoid unused import warnings if some helpers move later
var _ = sms.EnqueueInput{}
