package main

import "testing"

func TestCmdOpsHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		if err := cmdOps([]string{arg}); err != nil {
			t.Errorf("cmdOps(%q) = %v, want nil", arg, err)
		}
	}
}

func TestCmdOpsUnknownSubcommand(t *testing.T) {
	if err := cmdOps([]string{"bogus"}); err == nil {
		t.Error("cmdOps(bogus) = nil, want error")
	}
}
