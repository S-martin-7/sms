package horisen

import "fmt"

// Code is a Horisen SMS HTTP API result code.
// The canonical list: 100 = OK, 101 = OK with warnings, 102-116 = errors.
// Reference: https://developers.horisen.com/en/sms-http-api
type Code int

const (
	CodeOK           Code = 100
	CodeOKWithWarn   Code = 101
	CodeThrottled    Code = 105 // temporary rate limit — retry after backoff
)

// Error wraps a Horisen error code with a human-readable description.
type Error struct {
	Code        Code
	Description string
}

func (e *Error) Error() string {
	return fmt.Sprintf("horisen: code=%d %s", e.Code, e.Description)
}

// IsSuccess reports whether the code indicates a successful submission.
func IsSuccess(code Code) bool {
	return code == CodeOK || code == CodeOKWithWarn
}

// IsRetryable reports whether the code indicates a transient error that
// can be retried after backoff (e.g., throttling). Permanent errors
// (auth failures, invalid receiver, blacklisted content) return false
// so the caller stops trying and marks the message rejected.
//
// Transport-level errors (dial timeout, 5xx) are classified elsewhere by
// the caller and also count as retryable; this function only covers
// application-level Horisen codes.
func IsRetryable(code Code) bool {
	switch code {
	case CodeThrottled:
		return true
	default:
		return false
	}
}
