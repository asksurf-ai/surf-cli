package cli

import (
	"reflect"
	"testing"
)

func TestEagerArgs(t *testing.T) {
	reset(false)
	Root.PersistentFlags().Bool("json", false, "")

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "operation flag value starting with a dash",
			args: []string{"surf", "search-web", "--q", "-twitter bitcoin"},
			want: []string{"surf", "search-web"},
		},
		{
			name: "global flags after the value are kept",
			args: []string{"search-web", "--q", "-price", "--rsh-timeout", "5s", "-p", "prod"},
			want: []string{"search-web", "--rsh-timeout", "5s", "-p", "prod"},
		},
		{
			name: "operation flag with inline value",
			args: []string{"search-web", "--q=-Hyperliquid", "-H", "X-Test: 1"},
			want: []string{"search-web", "-H", "X-Test: 1"},
		},
		{
			name: "bool operation flag followed by a global flag",
			args: []string{"search-web", "--include-content", "-p", "prod", "--include-content", "--rsh-verbose"},
			want: []string{"search-web", "-p", "prod", "--rsh-verbose"},
		},
		{
			name: "known root bool flag keeps the next arg",
			args: []string{"search-web", "--json", "-pprod"},
			want: []string{"search-web", "-pprod"},
		},
		{
			name: "global flags are left alone",
			args: []string{"-p", "prod", "market-price", "-t", "5s", "--rsh-retry=1", "--help"},
			want: []string{"-p", "prod", "market-price", "-t", "5s", "--rsh-retry=1", "--help"},
		},
		{
			name: "stops at --",
			args: []string{"op", "--rsh-verbose", "--", "-t", "nope"},
			want: []string{"op", "--rsh-verbose"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EagerArgs(tc.args); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("EagerArgs(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
