package postgres

import "errors"

var (
	ErrVersionConflict     = errors.New("memory/postgres: version conflict")
	ErrIdempotencyConflict = errors.New("memory/postgres: idempotency conflict")
	ErrNotFound            = errors.New("memory/postgres: record not found")
	ErrSchemaVersionAhead  = errors.New("memory/postgres: schema version ahead of code")
	ErrRelayPublishFailed  = errors.New("memory/postgres: relay publish failed")
)
