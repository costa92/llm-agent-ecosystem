package contract

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RecordKindWorking  = "working"
	RecordKindEpisodic = "episodic"
	RecordKindSemantic = "semantic"
	DedupeCollapsedLoserIDMetadataKey = "dedupe_collapsed_loser_id"
)

var ErrInvalidRecordKind = errors.New("memory: invalid durable record kind")

// MemoryRecord is the backend-neutral durable record model used by pluggable
// storage implementations. It intentionally expresses durable-memory domain
// semantics without carrying SQL or backend-specific concerns.
type MemoryRecord struct {
	MemoryID                string
	TenantID                string
	UserID                  string
	ProjectID               string
	SessionID               string
	Kind                    string
	Source                  string
	Category                string
	Content                 string
	NormalizedContentHash   string
	Tags                    []string
	Importance              float64
	Pinned                  bool
	Disabled                bool
	Deleted                 bool
	Version                 int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
	DeletedAt               *time.Time
	LastAccessAt            *time.Time
	HitCount                int64
	ConsolidatedFromEventID string
}

type StoredEvent struct {
	Version        int64
	TenantID       string
	MemoryID       string
	EventType      string
	IdempotencyKey string
	Record         MemoryRecord
	Metadata       map[string]any
}

type OutboxMessage struct {
	Version   int64
	TenantID  string
	MemoryID  string
	EventType string
	EventID   string
	Record    MemoryRecord
	Metadata  map[string]any
}

type IdempotencyEntry struct {
	TenantID       string
	IdempotencyKey string
	RequestHash    string
	MemoryID       string
	Response       WriteRecordResult
	CreatedAt      time.Time
	ExpiresAt      *time.Time
}

type WriteRecordInput struct {
	TenantID       string
	IdempotencyKey string
	RequestHash    string
	Record         MemoryRecord
}

type WriteRecordResult struct {
	MemoryID string
	Version  int64
	Created  bool
	Record   MemoryRecord
}

type PatchRecordInput struct {
	TenantID        string
	MemoryID        string
	ExpectedVersion int64
	Content         string
	Category        string
	Tags            []string
	Importance      float64
}

type PatchRecordResult struct {
	MemoryID string
	Version  int64
	Record   MemoryRecord
}

type DeleteRecordInput struct {
	TenantID        string
	MemoryID        string
	ExpectedVersion int64
}

type DeleteRecordResult struct {
	MemoryID string
	Version  int64
	Record   MemoryRecord
}

type PinRecordInput struct {
	TenantID        string
	MemoryID        string
	ExpectedVersion int64
	Pinned          bool
}

type PinRecordResult struct {
	MemoryID string
	Version  int64
	Record   MemoryRecord
}

type DisableRecordInput struct {
	TenantID        string
	MemoryID        string
	ExpectedVersion int64
	Disabled        bool
}

type DisableRecordResult struct {
	MemoryID string
	Version  int64
	Record   MemoryRecord
}

type PromoteRecordInput struct {
	TenantID        string
	MemoryID        string
	ExpectedVersion int64
	SourceEventID   string
	IdempotencyKey  string
	Reason          string
}

type PromoteRecordResult struct {
	MemoryID string
	Version  int64
	Record   MemoryRecord
	Created  bool
}

type DedupeAction int

const (
	DedupeNoCollision DedupeAction = iota
	DedupeMergedExisting
	DedupeCollapsedByPin
)

type ResolveDedupeInput struct {
	TenantID  string
	DedupeKey string
	Candidate MemoryRecord
}

type ResolveDedupeResult struct {
	WinnerID string
	Action   DedupeAction
}

type MarkAccessInput struct {
	TenantID  string
	MemoryIDs []string
	AccessedAt time.Time
}

type RecordStore interface {
	GetRecord(ctx context.Context, tenantID, memoryID string) (MemoryRecord, error)
	WriteRecord(ctx context.Context, in WriteRecordInput) (WriteRecordResult, error)
	PatchRecord(ctx context.Context, in PatchRecordInput) (PatchRecordResult, error)
	DeleteRecord(ctx context.Context, in DeleteRecordInput) (DeleteRecordResult, error)
	PinRecord(ctx context.Context, in PinRecordInput) (PinRecordResult, error)
	DisableRecord(ctx context.Context, in DisableRecordInput) (DisableRecordResult, error)
}

type Promoter interface {
	Promote(ctx context.Context, in PromoteRecordInput) (PromoteRecordResult, error)
}

type Deduper interface {
	ResolveDedupe(ctx context.Context, in ResolveDedupeInput) (ResolveDedupeResult, error)
}

type AccessMarker interface {
	MarkAccess(ctx context.Context, in MarkAccessInput) error
}

type EventStore interface {
	AppendEvent(ctx context.Context, event StoredEvent) error
}

type IdempotencyStore interface {
	LoadIdempotency(ctx context.Context, tenantID, idempotencyKey string) (IdempotencyEntry, error)
	SaveIdempotency(ctx context.Context, entry IdempotencyEntry) error
}

type Outbox interface {
	EnqueueOutbox(ctx context.Context, msg OutboxMessage) error
}

type MessagePublisher interface {
	Publish(ctx context.Context, msg OutboxMessage) error
}

func NormalizeRecordKind(kind string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	if normalized == "" {
		return RecordKindEpisodic, nil
	}
	switch normalized {
	case RecordKindWorking, RecordKindEpisodic, RecordKindSemantic:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidRecordKind, kind)
	}
}

func (r MemoryRecord) NormalizeWriteDefaults() (MemoryRecord, error) {
	kind, err := NormalizeRecordKind(r.Kind)
	if err != nil {
		return MemoryRecord{}, err
	}
	r.Kind = kind
	return r, nil
}

func (r *MemoryRecord) SetWorkingDefault() {
	if r == nil {
		return
	}
	if strings.TrimSpace(r.Kind) == "" {
		r.Kind = RecordKindWorking
	}
}
