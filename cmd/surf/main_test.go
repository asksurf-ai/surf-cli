package main

import (
	"os"
	"testing"
)

func TestShouldInjectAPIName(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "operation triggers injection",
			args: []string{"surf", "wallet-labels-batch"},
			want: true,
		},
		{
			name: "operation with --help should NOT trigger injection",
			// Regression: injecting "surf" before the op name causes cobra
			// to render "Usage: surf surf wallet-labels-batch [flags]" because
			// the op is reached via the hidden "surf" API subcommand instead
			// of the lightweight stub on Root.
			args: []string{"surf", "wallet-labels-batch", "--help"},
			want: false,
		},
		{
			name: "operation with -h should NOT trigger injection",
			args: []string{"surf", "wallet-labels-batch", "-h"},
			want: false,
		},
		{
			name: "local command does not inject",
			args: []string{"surf", "auth"},
			want: false,
		},
		{
			name: "no args does not inject",
			args: []string{"surf"},
			want: false,
		},
		{
			name: "leading flags before operation still inject",
			args: []string{"surf", "-o", "json", "wallet-labels-batch"},
			want: true,
		},
		{
			name: "leading flags before operation with --help do NOT inject",
			args: []string{"surf", "-o", "json", "wallet-labels-batch", "--help"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = tt.args
			if got := shouldInjectAPIName(); got != tt.want {
				t.Errorf("shouldInjectAPIName() with args=%v = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestNeedsCachedAPI(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no command", []string{"surf"}, false},
		{"auth is meta", []string{"surf", "auth"}, false},
		{"sync is meta", []string{"surf", "sync"}, false},
		{"version is meta", []string{"surf", "version"}, false},
		{"install is meta", []string{"surf", "install"}, false},
		{"help is meta", []string{"surf", "help"}, false},
		{"catalog is meta", []string{"surf", "catalog", "show", "ethereum_dex_trades"}, false},
		{"telemetry is meta", []string{"surf", "telemetry"}, false},
		{"feedback is meta", []string{"surf", "feedback", "test"}, false},
		{"API command needs cache", []string{"surf", "polymarket-markets"}, true},
		{"list-operations needs cache", []string{"surf", "list-operations"}, true},
		{"API command with --help needs cache", []string{"surf", "polymarket-markets", "--help"}, true},
		{"leading flags skipped when finding command", []string{"surf", "--debug", "polymarket-markets"}, true},
		{"leading flags skipped before meta command", []string{"surf", "--debug", "auth"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()
			os.Args = tt.args
			if got := needsCachedAPI(); got != tt.want {
				t.Errorf("needsCachedAPI() with args=%v = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
