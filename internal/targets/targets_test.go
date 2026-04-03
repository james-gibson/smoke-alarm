package targets

import (
	"strings"
	"testing"
	"time"
)

func TestTargetValidateSuccess(t *testing.T) {
	target := validTarget()
	if err := target.Validate(); err != nil {
		t.Fatalf("expected valid target, got error: %v", err)
	}
}

func TestTargetValidateErrors(t *testing.T) {
	makeTarget := func() Target {
		return validTarget()
	}

	tests := []struct {
		name       string
		mutate     func(*Target)
		wantSubstr string
	}{
		{
			name: "missing id",
			mutate: func(tg *Target) {
				tg.ID = ""
			},
			wantSubstr: "target id is required",
		},
		{
			name: "missing endpoint for http transport",
			mutate: func(tg *Target) {
				tg.Endpoint = ""
			},
			wantSubstr: "endpoint is required",
		},
		{
			name: "invalid handshake profile",
			mutate: func(tg *Target) {
				tg.Check.HandshakeProfile = "extreme"
			},
			wantSubstr: "handshake_profile",
		},
		{
			name: "empty required method entry",
			mutate: func(tg *Target) {
				tg.Check.RequiredMethods = []string{"initialize", " "}
			},
			wantSubstr: "required_methods[1]",
		},
		{
			name: "hurl test missing both file and endpoint",
			mutate: func(tg *Target) {
				tg.Check.HURLTests = []HURLTest{
					{Name: "bad-hurl"},
				}
			},
			wantSubstr: "hurl_tests[0] requires either file or endpoint",
		},
		{
			name: "hurl test file and endpoint both set",
			mutate: func(tg *Target) {
				tg.Check.HURLTests = []HURLTest{
					{
						Name:     "conflict",
						File:     "test.hurl",
						Endpoint: "https://example.com/hurl",
					},
				}
			},
			wantSubstr: "hurl_tests[0] file and endpoint are mutually exclusive",
		},
		{
			name: "sse endpoint must use http or https",
			mutate: func(tg *Target) {
				tg.Transport = TransportSSE
				tg.Endpoint = "ws://example.com/events"
			},
			wantSubstr: "sse endpoint must use http/https",
		},
		{
			name: "stdio transport requires command",
			mutate: func(tg *Target) {
				tg.Transport = TransportStdio
				tg.Endpoint = ""
				tg.Stdio = StdioCommand{}
			},
			wantSubstr: "stdio command is required",
		},
		{
			name: "bearer auth requires secret",
			mutate: func(tg *Target) {
				tg.Auth = AuthConfig{
					Type: AuthBearer,
				}
			},
			wantSubstr: "bearer auth requires secret_ref",
		},
		{
			name: "oauth requires fields",
			mutate: func(tg *Target) {
				tg.Auth = AuthConfig{
					Type:     AuthOAuth,
					ClientID: "",
					TokenURL: "",
				}
			},
			wantSubstr: "oauth auth requires at least client_id and token_url",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			target := makeTarget()
			tc.mutate(&target)
			err := target.Validate()
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantSubstr)) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestCheckResultIsFailure(t *testing.T) {
	tests := []struct {
		state HealthState
		want  bool
	}{
		{StateHealthy, false},
		{StateDegraded, false},
		{StateUnhealthy, true},
		{StateOutage, true},
		{StateRegression, true},
	}
	for _, tc := range tests {
		cr := CheckResult{State: tc.state}
		if got := cr.IsFailure(); got != tc.want {
			t.Fatalf("IsFailure() for state %q = %v, want %v", tc.state, got, tc.want)
		}
	}
}

func TestCheckResultIsEscalated(t *testing.T) {
	tests := []struct {
		name   string
		result CheckResult
		want   bool
	}{
		{
			name: "critical severity escalates",
			result: CheckResult{
				State:    StateHealthy,
				Severity: SeverityCritical,
			},
			want: true,
		},
		{
			name: "regression state escalates",
			result: CheckResult{
				State: StateRegression,
			},
			want: true,
		},
		{
			name: "regression flag escalates",
			result: CheckResult{
				State:      StateHealthy,
				Regression: true,
			},
			want: true,
		},
		{
			name: "non-critical healthy does not escalate",
			result: CheckResult{
				State:    StateHealthy,
				Severity: SeverityInfo,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.IsEscalated(); got != tc.want {
				t.Fatalf("IsEscalated() = %v, want %v", got, tc.want)
			}
		})
	}
}

func validTarget() Target {
	return Target{
		ID:        "target-1",
		Enabled:   true,
		Protocol:  ProtocolMCP,
		Name:      "Example Target",
		Endpoint:  "https://example.com/mcp",
		Transport: TransportHTTP,
		Expected: ExpectedBehavior{
			HealthyStatusCodes: []int{200},
		},
		Auth: AuthConfig{
			Type: AuthNone,
		},
		Stdio: StdioCommand{},
		Check: CheckPolicy{
			Interval:         time.Second,
			Timeout:          time.Second,
			Retries:          1,
			HandshakeProfile: "base",
			RequiredMethods:  []string{"initialize"},
		},
		Tags: map[string]string{
			"env": "test",
		},
	}
}
