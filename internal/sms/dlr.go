package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DLR is the parsed shape of a Horisen DLR callback. Only fields we care
// about are pulled — extra keys (accountName, dlrTime, sendTime, ...) are
// preserved by passing the raw payload around when needed.
//
// Wire example (captured 2026-04-25):
//   {
//     "accountName":"16602_SamuelOTP",
//     "custom":{"msgId":"<our-uuid>","tenantId":1},
//     "event":"DELIVERED",
//     "errorMessage":"No error",
//     "msgId":"<horisen-uuid>",
//     "numParts":1,
//     "partNum":0
//   }
type DLR struct {
	HorisenMsgID string         `json:"msgId"`
	Event        string         `json:"event"`
	ErrorCode    json.RawMessage `json:"errorCode,omitempty"`   // can be int or string
	ErrorMessage string         `json:"errorMessage,omitempty"`
	NumParts     int            `json:"numParts,omitempty"`
	PartNum      int            `json:"partNum,omitempty"`
	Custom       struct {
		MsgID    string `json:"msgId"`
		TenantID int64  `json:"tenantId"`
	} `json:"custom"`
}

// ParseDLR decodes a Horisen DLR JSON body. Returns ErrInvalidDLR if the
// body is malformed or missing the fields needed to identify a message.
func ParseDLR(body []byte) (*DLR, error) {
	var d DLR
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDLR, err)
	}
	if d.Event == "" {
		return nil, fmt.Errorf("%w: missing event", ErrInvalidDLR)
	}
	if d.Custom.MsgID == "" && d.HorisenMsgID == "" {
		return nil, fmt.Errorf("%w: neither custom.msgId nor msgId present", ErrInvalidDLR)
	}
	return &d, nil
}

// DLRStatusFor maps a Horisen DLR `event` to our internal message status.
// Unknown events return ("", false) — caller should log and skip.
func DLRStatusFor(event string) (string, bool) {
	switch event {
	case "DELIVERED":
		return "delivered", true
	case "UNDELIVERED", "EXPIRED":
		return "undelivered", true
	case "REJECTED", "DELETED":
		return "rejected", true
	case "ACCEPTED", "BUFFERED", "ENROUTE":
		// transient — DLR is informational, no terminal transition yet.
		return "", false
	default:
		return "", false
	}
}

// ApplyDLRResult tells the caller what happened when a DLR was applied.
type ApplyDLRResult struct {
	MsgID      uuid.UUID
	TenantID   int64
	NewStatus  string
	Skipped    bool   // true if the DLR didn't trigger a transition (already final, unknown event)
	SkipReason string // populated when Skipped is true
}

// ApplyDLR locates the message a DLR refers to and updates its status.
// Lookup order:
//  1. By our internal UUID in custom.msgId (preferred — set on every send)
//  2. By Horisen's msgId on the message row (fallback for legacy messages)
//
// Returns ErrDLRMessageNotFound if no message matches either path.
func (s *Service) ApplyDLR(ctx context.Context, d *DLR) (*ApplyDLRResult, error) {
	newStatus, ok := DLRStatusFor(d.Event)
	if !ok {
		return &ApplyDLRResult{Skipped: true, SkipReason: "non-terminal or unknown event: " + d.Event}, nil
	}

	pgID, err := s.locateMessageID(ctx, d)
	if err != nil {
		return nil, err
	}

	var hMsgID *string
	if d.HorisenMsgID != "" {
		v := d.HorisenMsgID
		hMsgID = &v
	}
	var errCode, errMsg *string
	if code := dlrErrorCodeStr(d.ErrorCode); code != "" {
		errCode = &code
	}
	if d.ErrorMessage != "" && d.ErrorMessage != "No error" {
		v := d.ErrorMessage
		errMsg = &v
	}

	row, err := s.q.ApplyDLR(ctx, sqlcgen.ApplyDLRParams{
		ID:           pgID,
		Status:       newStatus,
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
		HorisenMsgID: hMsgID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Row exists but is in a final state already — late/duplicate DLR.
		return &ApplyDLRResult{
			MsgID:      uuid.UUID(pgID.Bytes),
			Skipped:    true,
			SkipReason: "message already in terminal state",
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("apply dlr: %w", err)
	}

	return &ApplyDLRResult{
		MsgID:     uuid.UUID(row.ID.Bytes),
		TenantID:  row.TenantID,
		NewStatus: row.Status,
	}, nil
}

// locateMessageID resolves the DB primary key for the message the DLR
// is about, trying custom.msgId first then horisen msgId.
func (s *Service) locateMessageID(ctx context.Context, d *DLR) (pgtype.UUID, error) {
	if d.Custom.MsgID != "" {
		if id, err := uuid.Parse(d.Custom.MsgID); err == nil {
			return pgtype.UUID{Bytes: id, Valid: true}, nil
		}
	}
	if d.HorisenMsgID != "" {
		v := d.HorisenMsgID
		row, err := s.q.GetMessageByHorisenMsgID(ctx, &v)
		if err == nil {
			return row.ID, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, fmt.Errorf("lookup by horisen msgId: %w", err)
		}
	}
	return pgtype.UUID{}, ErrDLRMessageNotFound
}

// dlrErrorCodeStr normalises Horisen's errorCode (which can be either a
// JSON number or a JSON string) into a string. Returns "" if absent or 0.
func dlrErrorCodeStr(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" || s == "0" {
			return ""
		}
		return s
	}
	// Then number.
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if n == "" || n == "0" {
			return ""
		}
		return n.String()
	}
	return ""
}
