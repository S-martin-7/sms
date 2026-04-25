package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/httpx"
	"github.com/S-martin-7/sms/internal/tenancy"
)

type adminAPIKeyResp struct {
	ID         int64      `json:"id"`
	TenantID   int64      `json:"tenant_id"`
	Prefix     string     `json:"prefix"`
	Name       *string    `json:"name,omitempty"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	// Token only present in the response of POST (issue) — never in GET.
	Token string `json:"token,omitempty"`
}

func toAdminKeyResp(k *tenancy.APIKey) adminAPIKeyResp {
	return adminAPIKeyResp{
		ID:         k.ID,
		TenantID:   k.TenantID,
		Prefix:     k.Prefix,
		Name:       k.Name,
		Scopes:     k.Scopes,
		LastUsedAt: k.LastUsedAt,
		RevokedAt:  k.RevokedAt,
		CreatedAt:  k.CreatedAt,
	}
}

// AdminListAPIKeysHandler — GET /admin/tenants/{id}/api-keys
func AdminListAPIKeysHandler(svc *tenancy.Service) http.HandlerFunc {
	type resp struct {
		Keys []adminAPIKeyResp `json:"keys"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		keys, err := svc.ListAPIKeys(r.Context(), tenantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "list failed")
			return
		}
		out := resp{Keys: make([]adminAPIKeyResp, 0, len(keys))}
		for _, k := range keys {
			out.Keys = append(out.Keys, toAdminKeyResp(k))
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// AdminIssueAPIKeyHandler — POST /admin/tenants/{id}/api-keys
// Returns the full token in the response — clients must store it
// immediately because it's never retrievable afterward.
func AdminIssueAPIKeyHandler(svc *tenancy.Service, audit *admin.Service, pepper string) http.HandlerFunc {
	type req struct {
		Name string `json:"name,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid tenant id")
			return
		}
		var in req
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json")
				return
			}
		}
		issued, err := svc.IssueAPIKey(r.Context(), tenantID, in.Name, pepper)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"api_key.issue", "api_key", strconv.FormatInt(issued.Record.ID, 10),
			map[string]any{"tenant_id": tenantID, "prefix": issued.Record.Prefix, "name": in.Name})

		out := toAdminKeyResp(issued.Record)
		out.Token = issued.Token
		httpx.WriteJSON(w, http.StatusCreated, out)
	}
}

// AdminRevokeAPIKeyHandler — POST /admin/api-keys/{id}/revoke
func AdminRevokeAPIKeyHandler(svc *tenancy.Service, audit *admin.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid key id")
			return
		}
		if err := svc.RevokeAPIKey(r.Context(), id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "revoke failed")
			return
		}
		_ = audit.LogAction(r.Context(), httpx.AdminIDFrom(r.Context()),
			"api_key.revoke", "api_key", strconv.FormatInt(id, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}
