package admin

import "errors"

var (
	ErrInvalidCredentials = errors.New("admin: invalid credentials")
	ErrAdminExists        = errors.New("admin: admin with that email already exists")
)
