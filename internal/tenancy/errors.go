package tenancy

import "errors"

var (
	ErrTenantNotFound  = errors.New("tenancy: tenant not found")
	ErrTenantSuspended = errors.New("tenancy: tenant suspended")
	ErrAPIKeyNotFound  = errors.New("tenancy: api key not found")
	ErrAPIKeyRevoked   = errors.New("tenancy: api key revoked")
	ErrAPIKeyInvalid   = errors.New("tenancy: api key invalid")
)
