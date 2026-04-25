package sms

import "errors"

var (
	ErrNotFound           = errors.New("sms: message not found")
	ErrDuplicateClientRef = errors.New("sms: client_ref already used for this tenant")
	ErrInvalidDLR         = errors.New("sms: invalid DLR payload")
	ErrDLRMessageNotFound = errors.New("sms: DLR refers to unknown message")
)
