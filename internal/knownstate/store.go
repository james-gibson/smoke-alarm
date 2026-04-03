package knownstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status represents normalized health outcomes for a target.
type Status string

const (
	StatusUnknown  Status = "unknown"
	StatusHealthy  Status = "healthy"
	StatusDegraded Status = "degraded"
	StatusOutage   Status = "outage"
	StatusFailed   Status = "failed"
)

// IsHealthy reports whether a status should be treated as healthy.
func IsHealthy(s Status) bool {
	return s == StatusHealthy
}

// IsFailure reports whether a status should be treated as a failure.
// Unknown is intentionally non-failing so startup/discovery noise doesn't
// immediately trigger failure semantics.
func IsFailure(s Status) bool {
	switch s {
	case StatusFailed, StatusOutage, StatusDegraded:
		return true
	default:
		return false
	}
}

// TargetState holds persisted baseline and transition memory for one target.
type TargetState struct {
	TargetID            string    `json:"target_id"`
	CurrentStatus       Status    `json:"current_status"`
	LastError           string    `json:"last_error,omitempty"`
	LastCheckedAt       time.Time `json:"last_checked_at"`
	LastHealthyAt       time.Time `json:"last_healthy_at,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	SuccessStreak       int       `json:"success_streak"`

	// EverHealthy means this target achieved sustained healthy baseline at least once.
	// Regression semantics hinge on this: if EverHealthy and a new failure appears,
	// classify as regression.
	EverHealthy bool `json:"ever_healthy"`
}

// Snapshot is the persisted known-state baseline file.
type Snapshot struct {
	SchemaVersion int                    `json:"schema_version"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Targets       map[string]TargetState `json:"targets"`
}

// UpdateInput is a single probe outcome to apply to known state.
type UpdateInput struct {
	TargetID  string
	Status    Status
	ErrorText string
	CheckedAt time.Time
}

// UpdateResult describes state transition semantics after applying an update.
type UpdateResult struct {
	Previous        TargetState
	Current         TargetState
	HadPrevious     bool
	IsRegression    bool
	BecameHealthy   bool
	BecameUnhealthy bool
}

// Store persists and evaluates known-state baseline semantics.
type Store struct {
	mu              sync.RWMutex
	path            string
	autoPersist     bool
	sustainSuccessN int
	now             func() time.Time
	snapshot        Snapshot
}

// Option customizes store behavior.
type Option func(*Store)

// WithAutoPersist toggles save-on-update.
func WithAutoPersist(v bool) Option {
	return func(s *Store) { s.autoPersist = v }
}

// WithSustainSuccess sets how many consecutive healthy observations are required
// before a target is considered baseline healthy.
func WithSustainSuccess(n int) Option {
	return func(s *Store) {
		if n < 1 {
			n = 1
		}
		s.sustainSuccessN = n
	}
}

// WithNow injects clock source (useful for tests).
func WithNow(now func() time.Time) Option {
	return func(s *Store) {
		if now != nil {
			s.now = now
		}
	}
}

// NewStore creates a known-state store for a JSON file path.
func NewStore(path string, opts ...Option) *Store {
	st := &Store{
		path:            path,
		autoPersist:     true,
		sustainSuccessN: 1,
		now:             time.Now,
		snapshot: Snapshot{
			SchemaVersion: 1,
			UpdatedAt:     time.Time{},
			Targets:       map[string]TargetState{},
		},
	}
	for _, opt := range opts {
		opt(st)
	}
	return st
}

// Path returns the backing file path.
func (s *Store) Path() string {
	return s.path
}

// Load reads snapshot from disk if present. Missing file is not an error.
func (s *Store) Load(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Fresh store: keep defaults.
			return nil
		}
		return fmt.Errorf("read known-state file: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("decode known-state file: %w", err)
	}
	if snap.SchemaVersion == 0 {
		snap.SchemaVersion = 1
	}
	if snap.Targets == nil {
		snap.Targets = map[string]TargetState{}
	}

	s.snapshot = snap
	return nil
}

// Save writes current snapshot atomically to disk.
func (s *Store) Save(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.RLock()
	snap := cloneSnapshot(s.snapshot)
	s.mu.RUnlock()

	snap.SchemaVersion = 1
	snap.UpdatedAt = s.now()

	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("encode known-state snapshot: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create known-state directory: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write temp known-state file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename known-state file: %w", err)
	}

	s.mu.Lock()
	s.snapshot.UpdatedAt = snap.UpdatedAt
	s.snapshot.SchemaVersion = snap.SchemaVersion
	s.mu.Unlock()

	return nil
}

// Snapshot returns a deep copy of current in-memory snapshot.
func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.snapshot)
}

// Get returns current state for a target.
func (s *Store) Get(targetID string) (TargetState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.snapshot.Targets[targetID]
	return t, ok
}

// Update applies one observation and computes regression semantics.
//
// Regression rule:
//   - If target has achieved EverHealthy and current observation is failure,
//     classify as regression.
func (s *Store) Update(ctx context.Context, in UpdateInput) (UpdateResult, error) {
	if err := ctx.Err(); err != nil {
		return UpdateResult{}, err
	}
	if in.TargetID == "" {
		return UpdateResult{}, errors.New("target id is required")
	}
	if in.Status == "" {
		in.Status = StatusUnknown
	}
	if in.CheckedAt.IsZero() {
		in.CheckedAt = s.now()
	}

	s.mu.Lock()
	prev, hadPrev := s.snapshot.Targets[in.TargetID]
	curr := prev
	if !hadPrev {
		curr = TargetState{
			TargetID:      in.TargetID,
			CurrentStatus: StatusUnknown,
		}
	}

	curr.TargetID = in.TargetID
	curr.CurrentStatus = in.Status
	curr.LastCheckedAt = in.CheckedAt
	curr.LastError = in.ErrorText

	// Transition accounting
	wasHealthy := IsHealthy(prev.CurrentStatus)
	isHealthy := IsHealthy(in.Status)
	isFailure := IsFailure(in.Status)

	if isHealthy {
		curr.SuccessStreak++
		curr.ConsecutiveFailures = 0

		// only promote to "ever healthy baseline" after sustained success
		if curr.SuccessStreak >= s.sustainSuccessN {
			curr.EverHealthy = true
			curr.LastHealthyAt = in.CheckedAt
		}
	} else {
		curr.SuccessStreak = 0
		if isFailure {
			curr.ConsecutiveFailures++
		} else {
			curr.ConsecutiveFailures = 0
		}
	}

	isRegression := curr.EverHealthy && isFailure

	s.snapshot.Targets[in.TargetID] = curr
	s.snapshot.SchemaVersion = 1
	s.snapshot.UpdatedAt = s.now()
	s.mu.Unlock()

	if s.autoPersist {
		if err := s.Save(ctx); err != nil {
			return UpdateResult{}, err
		}
	}

	return UpdateResult{
		Previous:        prev,
		Current:         curr,
		HadPrevious:     hadPrev,
		IsRegression:    isRegression,
		BecameHealthy:   !wasHealthy && isHealthy,
		BecameUnhealthy: wasHealthy && !isHealthy,
	}, nil
}

// Reset clears in-memory snapshot and optionally deletes on-disk file.
func (s *Store) Reset(ctx context.Context, deleteFile bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	s.snapshot = Snapshot{
		SchemaVersion: 1,
		UpdatedAt:     s.now(),
		Targets:       map[string]TargetState{},
	}
	s.mu.Unlock()

	if deleteFile {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove known-state file: %w", err)
		}
		return nil
	}

	return s.Save(ctx)
}

func cloneSnapshot(in Snapshot) Snapshot {
	out := Snapshot{
		SchemaVersion: in.SchemaVersion,
		UpdatedAt:     in.UpdatedAt,
		Targets:       make(map[string]TargetState, len(in.Targets)),
	}
	for k, v := range in.Targets {
		out.Targets[k] = v
	}
	return out
}
