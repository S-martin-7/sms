package admin

import (
	"context"
	"encoding/json"

	sqlcgen "github.com/S-martin-7/sms/internal/db/sqlc/generated"
)

// LogAction appends a row to audit_log. Best-effort: callers should NOT
// fail their operation if audit insert errors — instead we log it and
// move on. (The audit table is for forensics, not authorization.)
//
// actorID = 0 → audit row written with NULL actor (e.g. CLI/migration).
// metadata is marshalled to JSONB; pass nil for none.
func (s *Service) LogAction(ctx context.Context, actorID int64, action, targetType, targetID string, metadata any) error {
	var meta []byte
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		meta = b
	}
	params := sqlcgen.AppendAuditLogParams{
		Action: action,
	}
	if actorID != 0 {
		params.ActorID = &actorID
	}
	if targetType != "" {
		t := targetType
		params.TargetType = &t
	}
	if targetID != "" {
		t := targetID
		params.TargetID = &t
	}
	if meta != nil {
		params.Metadata = meta
	}
	return s.q.AppendAuditLog(ctx, params)
}
