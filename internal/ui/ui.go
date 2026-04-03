package ui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/james-gibson/smoke-alarm/internal/engine"
)

// SnapshotProvider is satisfied by the monitor engine and keeps the UI decoupled.
type SnapshotProvider interface {
	SnapshotStatuses() []engine.TargetRuntimeStatus
	SnapshotEvents() []engine.AlertEvent
	IsReady() bool
}

// Options tunes foreground dashboard behavior.
type Options struct {
	RefreshInterval time.Duration
	HeaderTitle     string
	MaxEvents       int
	ShowHelp        bool
	DemoMode        bool
}

// Dashboard is a Bubble Tea model for live monitor status.
type Dashboard struct {
	provider SnapshotProvider
	opts     Options

	width  int
	height int

	selected   int
	showEvents bool
	quitting   bool

	lastRefresh time.Time
	statuses    []engine.TargetRuntimeStatus
	events      []engine.AlertEvent
	ready       bool
	errText     string

	// Styles
	titleStyle     lipgloss.Style
	subtleStyle    lipgloss.Style
	healthyStyle   lipgloss.Style
	degradedStyle  lipgloss.Style
	unhealthyStyle lipgloss.Style
	outageStyle    lipgloss.Style
	regressStyle   lipgloss.Style
	selectedStyle  lipgloss.Style
	borderStyle    lipgloss.Style
}

type tickMsg time.Time
type loadMsg struct {
	at       time.Time
	ready    bool
	statuses []engine.TargetRuntimeStatus
	events   []engine.AlertEvent
	err      error
}

// NewDashboard creates a new Bubble Tea dashboard model.
func NewDashboard(provider SnapshotProvider, opts Options) Dashboard {
	if opts.RefreshInterval <= 0 {
		opts.RefreshInterval = 1 * time.Second
	}
	if opts.HeaderTitle == "" {
		opts.HeaderTitle = "OCD Smoke Alarm"
	}
	if opts.MaxEvents <= 0 {
		opts.MaxEvents = 8
	}

	return Dashboard{
		provider:   provider,
		opts:       opts,
		showEvents: true,

		titleStyle:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		subtleStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		healthyStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		degradedStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		unhealthyStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		outageStyle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")),
		regressStyle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		selectedStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")),
		borderStyle:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
	}
}

func (m Dashboard) Init() tea.Cmd {
	return tea.Batch(
		m.loadCmd(),
		m.tickCmd(),
	)
}

func (m Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.loadCmd(), m.tickCmd())

	case loadMsg:
		m.lastRefresh = msg.at
		m.ready = msg.ready
		m.errText = ""
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.statuses = msg.statuses
		m.events = msg.events
		if m.selected >= len(m.statuses) {
			m.selected = max(0, len(m.statuses)-1)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.statuses)-1 {
				m.selected++
			}
		case "tab":
			m.showEvents = !m.showEvents
		}
	}
	return m, nil
}

func (m Dashboard) View() string {
	var b strings.Builder

	if m.quitting {
		return "shutting down...\n"
	}

	header := fmt.Sprintf("%s  ready=%t  targets=%d  updated=%s",
		m.opts.HeaderTitle,
		m.ready,
		len(m.statuses),
		prettyAgo(m.lastRefresh),
	)
	b.WriteString(m.titleStyle.Render(header))
	b.WriteString("\n")

	if m.errText != "" {
		b.WriteString(m.outageStyle.Render("error: " + m.errText))
		b.WriteString("\n\n")
	}

	if len(m.statuses) == 0 {
		b.WriteString(m.subtleStyle.Render("No targets yet. Waiting for monitor data..."))
		b.WriteString("\n")
		return b.String()
	}

	table := m.renderStatusTable()
	b.WriteString(m.borderStyle.Render(table))
	b.WriteString("\n")

	topology := m.renderTopologyPane()
	b.WriteString("\n")
	b.WriteString(m.borderStyle.Render(topology))

	if m.opts.DemoMode {
		b.WriteString("\n")
		b.WriteString(m.borderStyle.Render(m.renderDemoStateMachine()))
	}

	if m.showEvents {
		events := m.renderEvents()
		b.WriteString("\n")
		b.WriteString(m.borderStyle.Render(events))
	}

	if m.opts.ShowHelp {
		b.WriteString("\n")
		help := "keys: ↑/k ↓/j select • tab toggle events • q quit"
		if m.opts.DemoMode {
			help += " • demo-state-machine: enabled"
		}
		b.WriteString(m.subtleStyle.Render(help))
	}
	b.WriteString("\n")

	return b.String()
}

// Run starts the foreground Bubble Tea dashboard.
func Run(ctx context.Context, provider SnapshotProvider, opts Options) error {
	model := NewDashboard(provider, opts)
	p := tea.NewProgram(model, tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

func (m Dashboard) tickCmd() tea.Cmd {
	interval := m.opts.RefreshInterval
	return func() tea.Msg {
		time.Sleep(interval)
		return tickMsg(time.Now())
	}
}

func (m Dashboard) loadCmd() tea.Cmd {
	return func() tea.Msg {
		now := time.Now()

		if m.provider == nil {
			return loadMsg{
				at:  now,
				err: fmt.Errorf("dashboard provider is nil"),
			}
		}

		statuses := m.provider.SnapshotStatuses()
		events := m.provider.SnapshotEvents()
		ready := m.provider.IsReady()

		sort.Slice(statuses, func(i, j int) bool {
			// Regressions/outages first, then by target id.
			ri := statuses[i].Regression || statuses[i].State == "regression" || statuses[i].State == "outage"
			rj := statuses[j].Regression || statuses[j].State == "regression" || statuses[j].State == "outage"
			if ri != rj {
				return ri
			}
			return statuses[i].TargetID < statuses[j].TargetID
		})

		maxE := m.opts.MaxEvents
		if maxE > 0 && len(events) > maxE {
			events = events[len(events)-maxE:]
		}

		return loadMsg{
			at:       now,
			ready:    ready,
			statuses: statuses,
			events:   events,
		}
	}
}

func (m Dashboard) renderStatusTable() string {
	var b strings.Builder
	b.WriteString("TARGET                  STATE       SEV       FAILURES  LAT(ms)  MESSAGE\n")
	b.WriteString("----------------------------------------------------------------------\n")

	for i, st := range m.statuses {
		state := string(st.State)
		sev := string(st.Severity)

		msg := st.Message
		if msg == "" {
			msg = "-"
		}
		msg = truncate(msg, max(20, m.width-70))

		row := fmt.Sprintf(
			"%-22s  %-10s %-8s %-8d  %-7d  %s",
			truncate(st.TargetID, 22),
			state,
			sev,
			st.ConsecutiveFailures,
			st.Latency.Milliseconds(),
			msg,
		)

		row = m.styleForState(st.State).Render(row)
		if i == m.selected {
			row = m.selectedStyle.Render("➤ " + row)
		} else {
			row = "  " + row
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	if len(m.statuses) > 0 {
		b.WriteString("\n")
		b.WriteString(m.renderSelectedDetails(m.statuses[m.selected]))
	}

	return b.String()
}

func (m Dashboard) renderSelectedDetails(st engine.TargetRuntimeStatus) string {
	lines := []string{
		fmt.Sprintf("selected: %s", st.TargetID),
		fmt.Sprintf("name: %s", fallback(st.Name, "-")),
		fmt.Sprintf("endpoint: %s", fallback(st.Endpoint, "-")),
		fmt.Sprintf("protocol: %s  state: %s  severity: %s", st.Protocol, st.State, st.Severity),
		fmt.Sprintf("last checked: %s  ever healthy: %t  regression: %t",
			st.LastCheckedAt.Format(time.RFC3339), st.EverHealthy, st.Regression),
	}
	if st.Message != "" {
		lines = append(lines, fmt.Sprintf("message: %s", st.Message))
	}
	return strings.Join(lines, "\n")
}

func (m Dashboard) renderEvents() string {
	var b strings.Builder
	b.WriteString("RECENT EVENTS\n")
	b.WriteString("-------------\n")

	if len(m.events) == 0 {
		b.WriteString(m.subtleStyle.Render("No events yet."))
		b.WriteString("\n")
		return b.String()
	}

	for _, e := range m.events {
		line := fmt.Sprintf(
			"%s  %-20s %-11s %-8s %s",
			e.CheckedAt.Format("15:04:05"),
			truncate(e.TargetID, 20),
			e.State,
			e.Severity,
			truncate(e.Message, max(24, m.width-64)),
		)
		line = m.styleForState(e.State).Render(line)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (m Dashboard) renderTopologyPane() string {
	type key struct {
		protocol  string
		transport string
		state     string
	}
	counts := map[key]int{}
	protocolCounts := map[string]int{}
	transportCounts := map[string]int{}

	for _, st := range m.statuses {
		proto := strings.ToLower(strings.TrimSpace(fmt.Sprint(st.Protocol)))
		if proto == "" {
			proto = "unknown"
		}
		transport := inferTransportFromEndpoint(st.Endpoint)
		state := strings.ToLower(strings.TrimSpace(fmt.Sprint(st.State)))
		if state == "" {
			state = "unknown"
		}

		protocolCounts[proto]++
		transportCounts[transport]++
		counts[key{protocol: proto, transport: transport, state: state}]++
	}

	var protocols []string
	for p := range protocolCounts {
		protocols = append(protocols, p)
	}
	sort.Strings(protocols)

	var transports []string
	for tr := range transportCounts {
		transports = append(transports, tr)
	}
	sort.Strings(transports)

	var b strings.Builder
	b.WriteString("TOPOLOGY & RELATIONSHIPS\n")
	b.WriteString("------------------------\n")
	b.WriteString(fmt.Sprintf("protocols:  %s\n", renderCountMap(protocolCounts)))
	b.WriteString(fmt.Sprintf("transports: %s\n", renderCountMap(transportCounts)))
	b.WriteString("\n")
	b.WriteString("protocol -> transport -> state\n")

	for _, p := range protocols {
		for _, tr := range transports {
			var states []string
			stateCounts := map[string]int{}
			for k, n := range counts {
				if k.protocol == p && k.transport == tr && n > 0 {
					stateCounts[k.state] = n
				}
			}
			if len(stateCounts) == 0 {
				continue
			}
			for s := range stateCounts {
				states = append(states, s)
			}
			sort.Strings(states)

			var stateParts []string
			for _, s := range states {
				stateParts = append(stateParts, fmt.Sprintf("%s=%d", s, stateCounts[s]))
			}
			b.WriteString(fmt.Sprintf("- %s -> %s -> %s\n", p, tr, strings.Join(stateParts, ", ")))
		}
	}

	if len(m.statuses) == 0 {
		b.WriteString(m.subtleStyle.Render("No relationship data yet."))
		b.WriteString("\n")
	}

	return b.String()
}

func (m Dashboard) renderDemoStateMachine() string {
	var healthy, degraded, unhealthy, outage, regression, unknown int
	for _, st := range m.statuses {
		switch strings.ToLower(strings.TrimSpace(fmt.Sprint(st.State))) {
		case "healthy":
			healthy++
		case "degraded":
			degraded++
		case "unhealthy", "failed":
			unhealthy++
		case "outage":
			outage++
		case "regression":
			regression++
		default:
			unknown++
		}
	}

	stateSummary := fmt.Sprintf(
		"healthy=%d  degraded=%d  unhealthy=%d  outage=%d  regression=%d  unknown=%d",
		healthy, degraded, unhealthy, outage, regression, unknown,
	)

	lines := []string{
		"DEMO MODE: STATE MACHINE",
		"------------------------",
		stateSummary,
		"",
		"          [DISCOVERED]",
		"               |",
		"               v",
		"          [VALIDATING]",
		"          /    |    \\",
		"         v     v     v",
		"   [HEALTHY] [DEGRADED] [UNHEALTHY]",
		"        |         |         |",
		"        |         |         v",
		"        |         |      [OUTAGE]",
		"        |         |         |",
		"        \\---------+---------/",
		"                  v",
		"             [REGRESSION]",
		"",
		"Legend: transitions are driven by safety checks + handshake outcomes",
	}
	return strings.Join(lines, "\n")
}

func renderCountMap(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

func inferTransportFromEndpoint(endpoint string) string {
	ep := strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case ep == "":
		return "unknown"
	case strings.HasPrefix(ep, "stdio://"):
		return "stdio"
	case strings.HasPrefix(ep, "wss://"), strings.HasPrefix(ep, "ws://"):
		return "websocket"
	case strings.HasPrefix(ep, "https://"), strings.HasPrefix(ep, "http://"):
		if strings.Contains(ep, "transport=sse") ||
			strings.Contains(ep, "accept=text/event-stream") ||
			strings.Contains(ep, "text/event-stream") ||
			strings.Contains(ep, "/sse") ||
			strings.Contains(ep, "/stream") ||
			strings.Contains(ep, "/events") {
			return "sse"
		}
		return "http"
	default:
		return "other"
	}
}

func (m Dashboard) styleForState(s any) lipgloss.Style {
	state := strings.ToLower(strings.TrimSpace(fmt.Sprint(s)))
	switch state {
	case "healthy":
		return m.healthyStyle
	case "degraded":
		return m.degradedStyle
	case "unhealthy", "failed":
		return m.unhealthyStyle
	case "outage":
		return m.outageStyle
	case "regression":
		return m.regressStyle
	default:
		return m.subtleStyle
	}
}

func fallback(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func prettyAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Optional helper for very lightweight manual foreground bring-up:
// go run ./cmd/ocd-smoke-alarm --mode=foreground ...
func debugMainIfNeeded() {
	if os.Getenv("OCD_SMOKE_ALARM_UI_DEBUG") == "" {
		return
	}
	_ = context.Background()
}
