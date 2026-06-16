package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestHiddenCompatRootFlags(t *testing.T) {
	root := &cobra.Command{Use: "surf"}
	addHiddenCompatRootFlags(root)

	f := root.PersistentFlags().Lookup("agent-view")
	if f == nil {
		t.Fatal("expected hidden --agent-view compatibility flag")
	}
	if !f.Hidden {
		t.Fatal("--agent-view should be hidden")
	}
}

func TestSearchWebQueryCompatAlias(t *testing.T) {
	cmd := &cobra.Command{
		Use: "search-web",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.Flags().String("q", "", "query")
	addHiddenCompatOperationFlags(cmd)

	f := cmd.Flags().Lookup("query")
	if f == nil {
		t.Fatal("expected hidden --query compatibility alias")
	}
	if !f.Hidden {
		t.Fatal("--query should be hidden")
	}

	cmd.SetArgs([]string{"--query", "bitcoin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	q, err := cmd.Flags().GetString("q")
	if err != nil {
		t.Fatalf("get q: %v", err)
	}
	if q != "bitcoin" {
		t.Fatalf("expected --query to populate --q, got %q", q)
	}
}
