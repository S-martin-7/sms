package httpapi

import (
	"encoding/csv"
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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contactResp struct {
	ID        int64           `json:"id"`
	TenantID  int64           `json:"tenant_id"`
	MSISDN    string          `json:"msisdn"`
	Name      *string         `json:"name,omitempty"`
	Notes     *string         `json:"notes,omitempty"`
	OptOut    bool            `json:"opt_out"`
	OptOutAt  *time.Time      `json:"opt_out_at,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func toContactResp(r sqlcgen.Contact) contactResp {
	out := contactResp{
		ID:        r.ID,
		TenantID:  r.TenantID,
		MSISDN:    r.Msisdn,
		Name:      r.Name,
		Notes:     r.Notes,
		OptOut:    r.OptOut,
		Metadata:  json.RawMessage(r.Metadata),
		CreatedAt: r.CreatedAt.Time,
		UpdatedAt: r.UpdatedAt.Time,
	}
	if r.OptOutAt.Valid {
		t := r.OptOutAt.Time
		out.OptOutAt = &t
	}
	return out
}

// AdminListContactsHandler — GET /admin/tenants/{id}/contacts
//   ?q= search   ?opt_out=true|false   ?list_id=N   ?limit= 1-200   ?cursor= int
func AdminListContactsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type resp struct {
		Contacts   []contactResp `json:"contacts"`
		NextCursor *string       `json:"next_cursor"`
		Total      int64         `json:"total"`
		OptedOut   int64         `json:"opted_out"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		query := r.URL.Query()
		params := sqlcgen.ListContactsParams{TenantID: tenantID, Lim: 50}
		if v := query.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 200 {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "limit must be 1-200")
				return
			}
			params.Lim = int32(n)
		}
		if v := query.Get("cursor"); v != "" {
			id, err := httpx.DecodeIntCursor(v)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
				return
			}
			params.CursorID = id
		}
		if v := strings.TrimSpace(query.Get("q")); v != "" {
			params.Q = &v
		}
		if v := query.Get("opt_out"); v != "" {
			b := v == "true" || v == "1"
			params.OptOut = &b
		}
		if v := query.Get("list_id"); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil && id > 0 {
				params.ListID = &id
			}
		}

		rows, err := q.ListContacts(r.Context(), params)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		counts, err := q.CountContactsByTenant(r.Context(), tenantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

		out := resp{
			Contacts: make([]contactResp, 0, len(rows)),
			Total:    counts.Total,
			OptedOut: counts.OptedOut,
		}
		for _, c := range rows {
			out.Contacts = append(out.Contacts, toContactResp(c))
		}
		if int32(len(rows)) == params.Lim {
			c := httpx.EncodeIntCursor(rows[len(rows)-1].ID)
			out.NextCursor = &c
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// AdminCreateContactHandler — POST /admin/tenants/{id}/contacts
func AdminCreateContactHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type req struct {
		MSISDN string `json:"msisdn"`
		Name   string `json:"name,omitempty"`
		Notes  string `json:"notes,omitempty"`
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
		in.MSISDN = strings.TrimSpace(in.MSISDN)
		if in.MSISDN == "" {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "msisdn required")
			return
		}
		row, err := q.CreateContact(r.Context(), sqlcgen.CreateContactParams{
			TenantID: tenantID,
			Msisdn:   in.MSISDN,
			Name:     ptrIfNotEmpty(in.Name),
			Notes:    ptrIfNotEmpty(in.Notes),
			Column5:  []byte(`{}`),
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				httpx.WriteError(w, http.StatusConflict, "duplicate", "this msisdn is already in the contact book")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"contact.create", "contact", strconv.FormatInt(row.ID, 10),
			map[string]any{"tenant_id": tenantID, "msisdn": in.MSISDN})
		httpx.WriteJSON(w, http.StatusCreated, toContactResp(row))
	}
}

// AdminContactOptOutHandler — POST /admin/contacts/{id}/opt-out  body {"opt_out":true|false}
func AdminContactOptOutHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type req struct {
		OptOut bool `json:"opt_out"`
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
		// Need tenant_id to satisfy the WHERE; look it up cheaply.
		var tenantID int64
		err := pool.QueryRow(r.Context(), `SELECT tenant_id FROM contacts WHERE id = $1`, id).Scan(&tenantID)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "contact not found")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if err := q.SetContactOptOut(r.Context(), sqlcgen.SetContactOptOutParams{
			ID: id, TenantID: tenantID, OptOut: in.OptOut,
		}); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		action := "contact.opt_in"
		if in.OptOut {
			action = "contact.opt_out"
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			action, "contact", strconv.FormatInt(id, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminDeleteContactHandler — DELETE /admin/contacts/{id}
func AdminDeleteContactHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		var tenantID int64
		err := pool.QueryRow(r.Context(), `SELECT tenant_id FROM contacts WHERE id = $1`, id).Scan(&tenantID)
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if err := q.DeleteContact(r.Context(), sqlcgen.DeleteContactParams{ID: id, TenantID: tenantID}); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"contact.delete", "contact", strconv.FormatInt(id, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminImportContactsCSVHandler — POST /admin/tenants/{id}/contacts/import (text/csv)
//
// CSV con header opcional. Columnas reconocidas (case-insensitive):
//   msisdn   — obligatoria; cualquiera de "msisdn","numero","number","phone"
//   name     — opcional; "name","nombre"
//   notes    — opcional; "notes","notas","comentario"
// Si no hay header, asume la primera columna es msisdn y la segunda es name.
func AdminImportContactsCSVHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
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
		// Parse with a generous limit — 5MB is ~250k rows of "569XX,Name".
		body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "read body: "+err.Error())
			return
		}
		records, err := readCSVFlexible(string(body))
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "parse csv: "+err.Error())
			return
		}
		if len(records) == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "csv has no rows")
			return
		}

		// Detect header row.
		msisdnCol, nameCol, notesCol := 0, 1, -1
		first := records[0]
		startRow := 0
		if looksLikeHeader(first) {
			startRow = 1
			msisdnCol, nameCol, notesCol = -1, -1, -1
			for i, h := range first {
				switch normalizeHeader(h) {
				case "msisdn", "numero", "number", "phone", "telefono":
					msisdnCol = i
				case "name", "nombre":
					nameCol = i
				case "notes", "notas", "comentario", "comentarios":
					notesCol = i
				}
			}
			if msisdnCol < 0 {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request",
					"no se encontró la columna 'msisdn' en el header")
				return
			}
		}

		out := resp{}
		for i := startRow; i < len(records); i++ {
			row := records[i]
			if msisdnCol >= len(row) {
				out.Skipped++
				continue
			}
			msisdn := strings.TrimSpace(row[msisdnCol])
			msisdn = strings.TrimPrefix(msisdn, "+")
			if msisdn == "" {
				out.Skipped++
				continue
			}
			var name, notes string
			if nameCol >= 0 && nameCol < len(row) {
				name = strings.TrimSpace(row[nameCol])
			}
			if notesCol >= 0 && notesCol < len(row) {
				notes = strings.TrimSpace(row[notesCol])
			}
			_, err := q.UpsertContact(r.Context(), sqlcgen.UpsertContactParams{
				TenantID: tenantID,
				Msisdn:   msisdn,
				Name:     ptrIfNotEmpty(name),
				Notes:    ptrIfNotEmpty(notes),
				Column5:  []byte(`{}`),
			})
			if err != nil {
				if len(out.Errors) < 20 {
					out.Errors = append(out.Errors, fmt.Sprintf("fila %d (%s): %v", i+1, msisdn, err))
				}
				out.Skipped++
				continue
			}
			out.Imported++
		}

		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"contact.import", "tenant", strconv.FormatInt(tenantID, 10),
			map[string]any{"imported": out.Imported, "skipped": out.Skipped})

		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// AdminListContactListsHandler — GET /admin/tenants/{id}/contact-lists
func AdminListContactListsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type listResp struct {
		ID          int64     `json:"id"`
		Name        string    `json:"name"`
		Description *string   `json:"description,omitempty"`
		MemberCount int64     `json:"member_count"`
		CreatedAt   time.Time `json:"created_at"`
	}
	type resp struct {
		Lists []listResp `json:"lists"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		rows, err := q.ListContactLists(r.Context(), tenantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		out := resp{Lists: make([]listResp, 0, len(rows))}
		for _, r := range rows {
			out.Lists = append(out.Lists, listResp{
				ID:          r.ID,
				Name:        r.Name,
				Description: r.Description,
				MemberCount: r.MemberCount,
				CreatedAt:   r.CreatedAt.Time,
			})
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// AdminCreateContactListHandler — POST /admin/tenants/{id}/contact-lists
func AdminCreateContactListHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type req struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
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
		if strings.TrimSpace(in.Name) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "name required")
			return
		}
		row, err := q.CreateContactList(r.Context(), sqlcgen.CreateContactListParams{
			TenantID:    tenantID,
			Name:        in.Name,
			Description: ptrIfNotEmpty(in.Description),
		})
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"contact_list.create", "contact_list", strconv.FormatInt(row.ID, 10),
			map[string]any{"tenant_id": tenantID, "name": row.Name})
		httpx.WriteJSON(w, http.StatusCreated, row)
	}
}

// AdminAddContactsToListHandler — POST /admin/contact-lists/{id}/members
//   { "contact_ids": [1,2,3] }
func AdminAddContactsToListHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	type req struct {
		ContactIDs []int64 `json:"contact_ids"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		listID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid list id")
			return
		}
		var tenantID int64
		err := pool.QueryRow(r.Context(), `SELECT tenant_id FROM contact_lists WHERE id = $1`, listID).Scan(&tenantID)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "list not found")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
		if len(in.ContactIDs) == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "contact_ids must be non-empty")
			return
		}
		if err := q.AddContactsToList(r.Context(), sqlcgen.AddContactsToListParams{
			Column1:    listID,
			TenantID:   tenantID,
			ContactIds: in.ContactIDs,
		}); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"contact_list.add_members", "contact_list", strconv.FormatInt(listID, 10),
			map[string]any{"count": len(in.ContactIDs)})
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminDeleteContactListHandler — DELETE /admin/contact-lists/{id}
func AdminDeleteContactListHandler(pool *pgxpool.Pool, audit *admin.Service) http.HandlerFunc {
	q := sqlcgen.New(pool)
	return func(w http.ResponseWriter, r *http.Request) {
		listID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid list id")
			return
		}
		var tenantID int64
		err := pool.QueryRow(r.Context(), `SELECT tenant_id FROM contact_lists WHERE id = $1`, listID).Scan(&tenantID)
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if err := q.DeleteContactList(r.Context(), sqlcgen.DeleteContactListParams{ID: listID, TenantID: tenantID}); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"contact_list.delete", "contact_list", strconv.FormatInt(listID, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// helpers --------------------------------------------------------------

func ptrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func looksLikeHeader(row []string) bool {
	if len(row) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(row[0]))
	switch first {
	case "msisdn", "numero", "number", "phone", "telefono", "name", "nombre":
		return true
	}
	// If first cell is mostly digits, definitely not a header.
	return false
}

func normalizeHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// strip BOM if present (common with Excel-saved CSVs)
	s = strings.TrimPrefix(s, "")
	return s
}

func readCSVFlexible(body string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(body))
	r.FieldsPerRecord = -1 // tolerate ragged rows
	r.LazyQuotes = true
	return r.ReadAll()
}
