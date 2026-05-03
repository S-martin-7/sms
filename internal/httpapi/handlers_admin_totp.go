package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/S-martin-7/sms/internal/admin"
	"github.com/S-martin-7/sms/internal/httpx"
)

type totpSetupResp struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"` // otpauth:// — render as QR
}

type totpCodeReq struct {
	Code string `json:"code"`
}

type meResp struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	TOTPEnabled bool   `json:"totp_enabled"`
}

// MeHandler returns the calling admin's profile (id, email, role,
// totp_enabled). Used by the dashboard to drive the 2FA settings UI.
func MeHandler(svc *admin.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := httpx.AdminIDFrom(r.Context())
		if adminID == 0 {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "no admin in context")
			return
		}
		u, err := svc.GetAdminByID(r.Context(), adminID)
		if err != nil {
			if errors.Is(err, admin.ErrAdminNotFound) {
				httpx.WriteError(w, http.StatusNotFound, "not_found", "admin not found")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "lookup failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, meResp{
			ID: u.ID, Email: u.Email, Role: u.Role, TOTPEnabled: u.TOTPEnabled,
		})
	}
}

// TOTPSetupHandler generates a fresh secret for the calling admin and
// returns it + the otpauth:// URI for QR rendering. The secret is
// stored on admin_users.totp_secret but totp_enabled stays false until
// the client follows up with TOTPEnableHandler.
func TOTPSetupHandler(svc *admin.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := httpx.AdminIDFrom(r.Context())
		if adminID == 0 {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "no admin in context")
			return
		}
		enr, err := svc.StartTOTPEnrollment(r.Context(), adminID)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, totpSetupResp{Secret: enr.Secret, URI: enr.URI})
	}
}

// TOTPEnableHandler turns on 2FA for the calling admin after verifying
// they can produce a valid code from the secret stored by
// TOTPSetupHandler.
func TOTPEnableHandler(svc *admin.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := httpx.AdminIDFrom(r.Context())
		if adminID == 0 {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "no admin in context")
			return
		}
		var in totpCodeReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Code == "" {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "code required")
			return
		}
		if err := svc.EnableTOTP(r.Context(), adminID, in.Code); err != nil {
			switch {
			case errors.Is(err, admin.ErrTOTPInvalid):
				httpx.WriteError(w, http.StatusUnauthorized, "totp_invalid", "code invalid")
			case errors.Is(err, admin.ErrTOTPNotEnrolled):
				httpx.WriteError(w, http.StatusBadRequest, "totp_not_enrolled", "call /admin/me/totp/setup first")
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "totp enable failed")
			}
			return
		}
		_ = svc.LogAction(r.Context(), adminID, "admin.totp_enabled", "admin_user", strconv.FormatInt(adminID, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminResetTOTPHandler — POST /admin/users/{id}/totp/reset.
//
// Superadmin-only escape hatch for an admin who lost their authenticator.
// Clears totp_enabled, totp_secret, failed_attempts, locked_until — the
// target user can log in with password only, then re-enroll from /cuenta.
//
// Why role-gated: any admin with a valid JWT could otherwise wipe another
// admin's 2FA. Limiting it to superadmin reduces blast radius.
func AdminResetTOTPHandler(svc *admin.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID := httpx.AdminIDFrom(r.Context())
		role := httpx.AdminRoleFrom(r.Context())
		if role != "superadmin" {
			httpx.WriteError(w, http.StatusForbidden, "forbidden",
				"only superadmin can reset another admin's 2FA")
			return
		}
		targetID, ok := parseInt64URLParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		if targetID == actorID {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request",
				"use /admin/me/totp/disable for self-reset")
			return
		}
		if err := svc.AdminResetTOTPAndLockout(r.Context(), targetID); err != nil {
			if errors.Is(err, admin.ErrAdminNotFound) {
				httpx.WriteError(w, http.StatusNotFound, "not_found", "admin not found")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "reset failed")
			return
		}
		_ = svc.LogAction(r.Context(), actorID, "admin.totp_reset", "admin_user",
			strconv.FormatInt(targetID, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminListAdminsHandler — GET /admin/users (superadmin only).
// Returns the admin roster so the superadmin recovery UI knows which id
// to target for /admin/users/{id}/totp/reset.
func AdminListAdminsHandler(svc *admin.Service) http.HandlerFunc {
	type item struct {
		ID          int64  `json:"id"`
		Email       string `json:"email"`
		Role        string `json:"role"`
		TOTPEnabled bool   `json:"totp_enabled"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if httpx.AdminRoleFrom(r.Context()) != "superadmin" {
			httpx.WriteError(w, http.StatusForbidden, "forbidden",
				"superadmin role required")
			return
		}
		users, err := svc.ListAdmins(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "list failed")
			return
		}
		out := make([]item, 0, len(users))
		for _, u := range users {
			out = append(out, item{ID: u.ID, Email: u.Email, Role: u.Role, TOTPEnabled: u.TOTPEnabled})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": out})
	}
}

// TOTPDisableHandler turns 2FA off after verifying a current code.
// Lost-authenticator recovery is intentionally NOT supported here —
// it must be done by a superadmin via a future reset endpoint to
// avoid a "forgot phone, disabled 2FA from existing session" attack.
func TOTPDisableHandler(svc *admin.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := httpx.AdminIDFrom(r.Context())
		if adminID == 0 {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "no admin in context")
			return
		}
		var in totpCodeReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Code == "" {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "code required")
			return
		}
		if err := svc.DisableTOTP(r.Context(), adminID, in.Code); err != nil {
			switch {
			case errors.Is(err, admin.ErrTOTPInvalid):
				httpx.WriteError(w, http.StatusUnauthorized, "totp_invalid", "code invalid")
			case errors.Is(err, admin.ErrTOTPNotEnrolled):
				httpx.WriteError(w, http.StatusBadRequest, "totp_not_enrolled", "totp is not active")
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "totp disable failed")
			}
			return
		}
		_ = svc.LogAction(r.Context(), adminID, "admin.totp_disabled", "admin_user", strconv.FormatInt(adminID, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}
