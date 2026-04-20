package tenancy

import "errors"

var (
	ErrTenantNotFound = errors.New("tenancy: tenant not found")
	ErrAPIKeyNotFound = errors.New("tenancy: api key not found")
	ErrAPIKeyRevoked  = errors.New("tenancy: api key revoked")
	ErrAPIKeyInvalid  = errors.New("tenancy: api key invalid")
)
