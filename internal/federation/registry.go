package federation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// EventType represents the kind of registry change that occurred.
type EventType string

const (
	EventAdded   EventType = "added"
	EventUpdated EventType = "updated"
	EventRemoved EventType = "removed"
)

const (
	registrySnapshotFile = "registry.json"
)

// InstanceRecord captures the federation-relevant details of a running service instance.
type InstanceRecord struct {
	ID          string            `json:"id"`
	ServiceName string            `json:"service_name"`
	Hostname    string            `json:"hostname"`
	Port        int               `json:"port"`
	Role        Role              `json:"role"`
	PID         int               `json:"pid"`
	StartedAt   time.Time         `json:"started_at"`
	AnnouncedAt time.Time         `json:"announced_at"`
	LastSeenAt  time.Time         `json:"last_seen_at"`
	Introducer  string            `json:"introducer"`
	Upstream    string            `json:"upstream,omitempty"`
	Downstream  []string          `json:"downstream,omitempty"`
	Meta        map[string]string `json:"meta,omitempty"`
}

// Event describes a change in the registry that may be surfaced to observers.
type Event struct {
	Type    EventType      `json:"type"`
	Record  InstanceRecord `json:"record"`
	Reason  string         `json:"reason,omitempty"`
	At      time.Time      `json:"at"`
	Version uint64         `json:"version"`
}

// Snapshot contains the current introducer identity and peer membership view.
type Snapshot struct {
	Self         InstanceRecord   `json:"self"`
	IntroducerID string           `json:"introducer_id"`
	Peers        []InstanceRecord `json:"peers"`
	Version      uint64           `json:"version"`
	GeneratedAt  time.Time        `json:"generated_at"`
}

// RegistryOptions tunes the behaviour of the federation registry.
type RegistryOptions struct {
	StateDir         string
	AnnounceInterval time.Duration
	HeartbeatTimeout time.Duration
	MaxPeers         int
	OnChange         func(Event)
}

// registryEntry keeps runtime-only tracking data.
type registryEntry struct {
	record InstanceRecord
}

type Registry struct {
	mu         sync.RWMutex
	snapshotMu sync.Mutex

	self Identity

	opts RegistryOptions

	version uint64
	peers   map[string]registryEntry
}

// NewRegistry creates a registry rooted at the provided identity.
func NewRegistry(self Identity, opts RegistryOptions) (*Registry, error) {
	if self.ID == "" {
		return nil, errors.New("federation: registry requires identity with ID")
	}
	if opts.AnnounceInterval <= 0 {
		opts.AnnounceInterval = 10 * time.Second
	}
	if opts.HeartbeatTimeout <= 0 {
		opts.HeartbeatTimeout = 45 * time.Second
	}
	if opts.MaxPeers <= 0 {
		opts.MaxPeers = 256
	}
	r := &Registry{
		self:    self,
		opts:    opts,
		version: 1,
		peers:   make(map[string]registryEntry, 32),
	}
	return r, nil
}

// Self returns the local instance record for use in announcements.
func (r *Registry) Self() InstanceRecord {
	return InstanceRecord{
		ID:          r.self.ID,
		ServiceName: r.self.ServiceName,
		Hostname:    r.self.Hostname,
		Port:        r.self.Port,
		Role:        r.self.Role,
		PID:         r.self.PID,
		StartedAt:   r.self.CreatedAt,
		AnnouncedAt: time.Now().UTC(),
		LastSeenAt:  time.Now().UTC(),
		Introducer:  r.self.ID,
	}
}

// Upsert introduces or refreshes a peer record.
func (r *Registry) Upsert(rec InstanceRecord, reason string) Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ignore self-record updates; handled separately.
	if rec.ID == "" || rec.ID == r.self.ID {
		return Event{}
	}

	now := time.Now().UTC()
	rec.LastSeenAt = now
	if rec.AnnouncedAt.IsZero() {
		rec.AnnouncedAt = now
	}
	if rec.StartedAt.IsZero() {
		rec.StartedAt = now
	}

	entry, exists := r.peers[rec.ID]

	if !exists && len(r.peers) >= r.opts.MaxPeers {
		return Event{} // refuse to add beyond cap
	}

	r.version++
	entry.record = rec
	r.peers[rec.ID] = entry

	eventType := EventUpdated
	if !exists {
		eventType = EventAdded
	}
	event := Event{
		Type:    eventType,
		Record:  rec,
		Reason:  reason,
		At:      now,
		Version: r.version,
	}
	if r.opts.OnChange != nil {
		r.opts.OnChange(event)
	}
	return event
}

// Remove deletes a peer from the registry.
func (r *Registry) Remove(peerID, reason string) Event {
	if peerID == "" || peerID == r.self.ID {
		return Event{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.peers[peerID]
	if !exists {
		return Event{}
	}
	delete(r.peers, peerID)
	r.version++

	event := Event{
		Type:    EventRemoved,
		Record:  entry.record,
		Reason:  reason,
		At:      time.Now().UTC(),
		Version: r.version,
	}
	if r.opts.OnChange != nil {
		r.opts.OnChange(event)
	}
	return event
}

// AgeOut removes peers that have not sent a heartbeat inside the allowed timeout.
func (r *Registry) AgeOut() []Event {
	timeout := r.opts.HeartbeatTimeout
	if timeout <= 0 {
		return nil
	}

	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()

	var events []Event
	for id, entry := range r.peers {
		if now.Sub(entry.record.LastSeenAt) < timeout {
			continue
		}
		delete(r.peers, id)
		r.version++
		ev := Event{
			Type:    EventRemoved,
			Record:  entry.record,
			Reason:  "heartbeat_timeout",
			At:      now,
			Version: r.version,
		}
		events = append(events, ev)
		if r.opts.OnChange != nil {
			r.opts.OnChange(ev)
		}
	}
	return events
}

// Snapshot captures a consistent view of the membership.
func (r *Registry) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peers := make([]InstanceRecord, 0, len(r.peers))
	for _, entry := range r.peers {
		peers = append(peers, entry.record)
	}
	slices.SortFunc(peers, func(a, b InstanceRecord) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})

	introducerID := r.self.ID
	for _, entry := range peers {
		if entry.Role == RoleIntroducer {
			introducerID = entry.ID
			break
		}
	}

	return Snapshot{
		Self:         r.Self(),
		IntroducerID: introducerID,
		Peers:        peers,
		Version:      r.version,
		GeneratedAt:  time.Now().UTC(),
	}
}

// SaveSnapshot writes the current registry snapshot to disk for observability.
func (r *Registry) SaveSnapshot() error {
	if r.opts.StateDir == "" {
		return nil
	}

	r.snapshotMu.Lock()
	defer r.snapshotMu.Unlock()

	snap := r.Snapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(r.opts.StateDir, slotStateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, registrySnapshotFile+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	finalPath := filepath.Join(dir, registrySnapshotFile)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
