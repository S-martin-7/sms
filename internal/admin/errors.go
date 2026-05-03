package admin

import "errors"

var (
	ErrInvalidCredentials = errors.New("admin: invalid credentials")
	ErrAdminExists        = errors.New("admin: admin with that email already exists")
	ErrTOTPRequired       = errors.New("admin: totp code required")
	ErrTOTPInvalid        = errors.New("admin: totp code invalid")
	ErrTOTPNotEnrolled    = errors.New("admin: totp not enrolled for this user")
	ErrAdminNotFound      = errors.New("admin: admin not found")
)
