package postgres

import (
	"context"
	"fmt"
	"time"

	corememory "github.com/costa92/llm-agent-memory/memory"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Publisher interface {
	Publish(ctx context.Context, evt corememory.OutboxMessage) error
}

type outboxExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type claimedOutboxMessage struct {
	OutboxID string
	Payload  corememory.OutboxMessage
}

var _ corememory.MessagePublisher = (Publisher)(nil)

type Relay struct {
	store     *Store
	publisher Publisher
	batchSize int
}

type RunStats struct {
	Published int
	Failed    int
}

func NewRelay(store *Store, publisher Publisher, batchSize int) (*Relay, error) {
	if store == nil {
		return nil, fmt.Errorf("memory/postgres: relay store is required")
	}
	if publisher == nil {
		return nil, fmt.Errorf("%w: publisher is required", ErrRelayPublishFailed)
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	return &Relay{store: store, publisher: publisher, batchSize: batchSize}, nil
}

func (r *Relay) RunOnce(ctx context.Context) (RunStats, error) {
	tx, claimed, err := r.claimPending(ctx)
	if err != nil {
		return RunStats{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	stats := RunStats{}
	for _, msg := range claimed {
		outboxID := msg.OutboxID
		payload := msg.Payload
		if err := r.publisher.Publish(ctx, payload); err != nil {
			stats.Failed++
			if markErr := r.markOutboxFailed(ctx, tx, outboxID, err); markErr != nil {
				return stats, markErr
			}
			continue
		}
		stats.Published++
		if err := r.markOutboxSent(ctx, tx, outboxID); err != nil {
			return stats, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return stats, fmt.Errorf("memory/postgres: commit relay tx: %w", err)
	}
	return stats, nil
}

func (r *Relay) claimPending(ctx context.Context) (pgx.Tx, []claimedOutboxMessage, error) {
	tx, err := r.store.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("memory/postgres: begin relay claim tx: %w", err)
	}

	rows, err := tx.Query(ctx,
		fmt.Sprintf(`SELECT outbox_id, payload
FROM %s
WHERE status = $1
ORDER BY created_at
FOR UPDATE SKIP LOCKED
LIMIT $2`, r.store.outboxTable()),
		outboxStatusPending, r.batchSize,
	)
	if err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		return nil, nil, fmt.Errorf("memory/postgres: relay query pending: %w", err)
	}
	defer rows.Close()

	claimed := make([]claimedOutboxMessage, 0, r.batchSize)
	for rows.Next() {
		var outboxID string
		var raw []byte
		if err := rows.Scan(&outboxID, &raw); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return nil, nil, fmt.Errorf("memory/postgres: relay scan pending: %w", err)
		}

		var payload corememory.OutboxMessage
		if err := decodeJSON(raw, &payload); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return nil, nil, fmt.Errorf("memory/postgres: relay decode payload: %w", err)
		}
		claimed = append(claimed, claimedOutboxMessage{
			OutboxID: outboxID,
			Payload:  payload,
		})
	}
	if err := rows.Err(); err != nil {
		tx.Rollback(ctx) //nolint:errcheck
		return nil, nil, fmt.Errorf("memory/postgres: relay rows: %w", err)
	}
	return tx, claimed, nil
}

func (r *Relay) markOutboxSent(ctx context.Context, execer outboxExecer, outboxID string) error {
	_, err := execer.Exec(ctx,
		fmt.Sprintf(`UPDATE %s SET status = $1, sent_at = $2 WHERE outbox_id = $3`, r.store.outboxTable()),
		outboxStatusSent, time.Now().UTC(), outboxID,
	)
	if err != nil {
		return fmt.Errorf("memory/postgres: mark outbox sent: %w", err)
	}
	return nil
}

func (r *Relay) markOutboxFailed(ctx context.Context, execer outboxExecer, outboxID string, publishErr error) error {
	_, err := execer.Exec(ctx,
		fmt.Sprintf(`UPDATE %s SET status = $1, attempt_count = attempt_count + 1, last_error = $2 WHERE outbox_id = $3`, r.store.outboxTable()),
		outboxStatusPending, publishErr.Error(), outboxID,
	)
	if err != nil {
		return fmt.Errorf("memory/postgres: mark outbox failed: %w", err)
	}
	return nil
}

type MemoryPublisher struct {
	Events []corememory.OutboxMessage
	Fail   bool
}

func (p *MemoryPublisher) Publish(ctx context.Context, evt corememory.OutboxMessage) error {
	_ = ctx
	if p.Fail {
		return ErrRelayPublishFailed
	}
	p.Events = append(p.Events, evt)
	return nil
}

type rowIter interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

var _ rowIter = (pgx.Rows)(nil)
