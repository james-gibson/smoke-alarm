package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultBasePort is the lowest local port considered for federation slots
	// when no explicit value is supplied.
	DefaultBasePort = 19100
	// DefaultSlotCount controls how many sequential ports are probed starting
	// at DefaultBasePort if no explicit max port is provided.
	DefaultSlotCount = 4

	slotStateDirName = "federation"
	slotIdentityFile = "identity.json"
	slotLockFile     = "slot.lock"
)

// Role identifies the local federation role once a slot is claimed.
type Role string

const (
	RoleIntroducer Role = "introducer"
	RoleFollower   Role = "follower"
)

// ErrNoFreeSlots is returned when every candidate port in the requested range
// is already occupied by another process.
var ErrNoFreeSlots = errors.New("federation: no free local slots available")

// SlotOptions controls how local federation slots are claimed.
type SlotOptions struct {
	// BasePort is the first port to try when probing for an open slot.
	// Defaults to DefaultBasePort.
	BasePort int
	// MaxPort is the final port (inclusive) to probe. If zero, an implicit
	// range of [BasePort, BasePort+DefaultSlotCount) is used.
	MaxPort int
	// StateDir contains runtime state; federation identity files live inside
	// <StateDir>/federation/.
	StateDir string
	// ServiceName is used when deriving stable instance IDs.
	ServiceName string
}

// Identity captures the elected slot and leader/follower role.
type Identity struct {
	ID          string    `json:"id"`
	ServiceName string    `json:"service_name"`
	Hostname    string    `json:"hostname"`
	StateDir    string    `json:"state_dir"`
	Port        int       `json:"port"`
	Role        Role      `json:"role"`
	PID         int       `json:"pid"`
	CreatedAt   time.Time `json:"created_at"`
}

// SlotClaim represents a reserved local federation slot.
type SlotClaim struct {
	Listener net.Listener
	Identity Identity
}

// Close releases the bound listener associated with the slot.
func (c *SlotClaim) Close() error {
	if c == nil || c.Listener == nil {
		return nil
	}
	return c.Listener.Close()
}

// ClaimSlot attempts to bind to a deterministic local port and returns the
// resulting identity + listener. The lowest available port in the configured
// range is always selected, so the first successful process becomes the
// introducer.
func ClaimSlot(opts SlotOptions) (*SlotClaim, error) {
	opts = normalizeOptions(opts)

	slotDir := filepath.Join(opts.StateDir, slotStateDirName)
	if err := os.MkdirAll(slotDir, 0o755); err != nil {
		return nil, fmt.Errorf("federation: create slot directory: %w", err)
	}

	lockPath := filepath.Join(slotDir, slotLockFile)
	lockFile, err := acquireSlotLock(lockPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = lockFile.Close()
		_ = os.Remove(lockPath)
	}()

	identityPath := filepath.Join(slotDir, slotIdentityFile)
	prevIdentity, _ := loadIdentity(identityPath)

	candidates := candidatePorts(opts.BasePort, opts.MaxPort, prevIdentity)
	var listener net.Listener
	var chosenPort int
	var chosenRole Role

	for _, port := range candidates {
		addr := fmt.Sprintf("localhost:%d", port)
		var lc net.ListenConfig
		ln, listenErr := lc.Listen(context.Background(), "tcp", addr)
		if listenErr != nil {
			continue
		}
		listener = ln
		chosenPort = port
		if port == opts.BasePort {
			chosenRole = RoleIntroducer
		} else {
			chosenRole = RoleFollower
		}
		break
	}

	if listener == nil {
		return nil, ErrNoFreeSlots
	}

	hostname, _ := os.Hostname()
	identity := Identity{
		ID:          generateInstanceID(hostname, opts.ServiceName, opts.StateDir, chosenPort),
		ServiceName: opts.ServiceName,
		Hostname:    hostname,
		StateDir:    opts.StateDir,
		Port:        chosenPort,
		Role:        chosenRole,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}

	if err := persistIdentity(identityPath, identity); err != nil {
		_ = listener.Close()
		return nil, err
	}

	return &SlotClaim{
		Listener: listener,
		Identity: identity,
	}, nil
}

func normalizeOptions(opts SlotOptions) SlotOptions {
	if opts.BasePort <= 0 || opts.BasePort > 65535 {
		opts.BasePort = DefaultBasePort
	}
	if opts.MaxPort <= 0 || opts.MaxPort < opts.BasePort || opts.MaxPort > 65535 {
		opts.MaxPort = opts.BasePort + DefaultSlotCount - 1
	}
	if opts.StateDir == "" {
		opts.StateDir = "./state"
	}
	if opts.ServiceName == "" {
		opts.ServiceName = "ocd-smoke-alarm"
	}
	return opts
}

func candidatePorts(base, maxPort int, previous *Identity) []int {
	total := maxPort - base + 1
	ports := make([]int, 0, total)

	if previous != nil && previous.Port >= base && previous.Port <= maxPort {
		ports = append(ports, previous.Port)
	}

	for port := base; port <= maxPort; port++ {
		if previous != nil && port == previous.Port {
			continue
		}
		ports = append(ports, port)
	}

	return ports
}

func loadIdentity(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ident Identity
	if err := json.Unmarshal(data, &ident); err != nil {
		return nil, err
	}
	return &ident, nil
}

func persistIdentity(path string, ident Identity) error {
	tmpPath := path + ".tmp"
	data, err := json.MarshalIndent(ident, "", "  ")
	if err != nil {
		return fmt.Errorf("federation: marshal identity: %w", err)
	}
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("federation: write temp identity: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("federation: replace identity: %w", err)
	}
	return nil
}

func generateInstanceID(hostname, service, stateDir string, port int) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(hostname)),
		strings.ToLower(strings.TrimSpace(service)),
		filepath.Clean(stateDir),
		strconv.Itoa(port),
		runtime.GOOS,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])[:16]
}

func acquireSlotLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
		return f, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("federation: acquire slot lock: %w", err)
	}

	pid, readErr := readPID(path)
	if readErr != nil || !processAlive(pid) {
		_ = os.Remove(path)
		f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			return f, nil
		}
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("federation: slot lock busy (pid %d)", pid)
		}
		return nil, fmt.Errorf("federation: acquire slot lock after cleanup: %w", err)
	}

	return nil, fmt.Errorf("federation: another process is claiming slots (pid %d)", pid)
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return 0, errors.New("empty pid")
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
