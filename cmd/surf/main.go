package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cyberconnecthq/surf-cli/cli"
	"github.com/cyberconnecthq/surf-cli/openapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//go:embed apis.json
var embeddedAPIsJSON []byte

var version = "dev"

func main() {
	// Force config and cache to ~/.config/surf/ on all platforms.
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	configDir := filepath.Join(home, ".config", "surf")
	os.Setenv("SURF_CONFIG_DIR", configDir)
	os.Setenv("SURF_CACHE_DIR", configDir)

	// Ensure config directory exists.
	if err := os.MkdirAll(configDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create config dir: %v\n", err)
		os.Exit(1)
	}

	// Bootstrap apis.json from embedded config on first run.
	apisPath := filepath.Join(configDir, "apis.json")
	if _, err := os.Stat(apisPath); os.IsNotExist(err) {
		if err := os.WriteFile(apisPath, embeddedAPIsJSON, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot write apis.json: %v\n", err)
			os.Exit(1)
		}
	}

	// Inject "surf" as the API name into os.Args so restish's Run() loads
	// the API config. Skip injection for commands that don't need API loading.
	//   surf market-futures --symbol BTC  →  [surf, surf, market-futures, --symbol, BTC]
	//   surf login                        →  [surf, login]  (no injection)
	if shouldInjectAPIName() {
		os.Args = append([]string{os.Args[0], "surf"}, os.Args[1:]...)
	}

	// Initialize restish.
	cli.Init("surf", version)
	cli.Defaults()
	cli.AddLoader(openapi.New())

	// Customize root command.
	cli.Root.Use = "surf"
	cli.Root.Short = "Surf data platform CLI"
	cli.Root.Long = "Query the Surf data platform — crypto market data, on-chain analytics, and more."
	cli.Root.Example = "  surf market-futures --symbol BTC\n  surf search-project --q bitcoin"
	// Override restish's default root behavior (acts as GET handler with MinimumNArgs(1)).
	cli.Root.Args = nil
	cli.Root.Run = func(cmd *cobra.Command, args []string) {
		cmd.Help()
	}
	cli.Root.ValidArgsFunction = nil

	// Remove restish generic commands we don't need.
	removeCommands(cli.Root,
		"head", "options", "get", "post", "put", "patch", "delete",
		"edit", "api", "links", "cert", "auth-header", "bulk",
	)

	// Hide the intermediate "surf" API subcommand — users call commands
	// directly (surf market-futures), not via a nested subcommand (surf surf market-futures).
	for _, cmd := range cli.Root.Commands() {
		if cmd.Use == "surf" {
			cmd.Hidden = true
			break
		}
	}

	// Populate "Available API Commands" in help from cached API spec.
	// These are lightweight stubs for display only — real execution goes
	// through the "surf" subcommand via shouldInjectAPIName().
	if api := cli.LoadCachedAPI("surf"); api != nil {
		for _, op := range api.Operations {
			if op.Hidden {
				continue
			}
			cli.Root.AddCommand(&cobra.Command{
				Use:     op.Name,
				GroupID: "api",
				Short:   op.Short,
				Aliases: op.Aliases,
				Run: func(cmd *cobra.Command, args []string) {
					cmd.Help()
				},
			})
		}
	}

	// Add custom commands directly on Root (not under the API subcommand).
	cli.Root.AddCommand(newSyncCmd())
	cli.Root.AddCommand(newVersionCmd())
	cli.Root.AddCommand(newInstallCmd())
	cli.Root.AddCommand(newListOperationsCmd())

	// Run.
	if err := cli.Run(); err != nil {
		os.Exit(1)
	}
	os.Exit(cli.GetExitCode())
}

// shouldInjectAPIName returns true if os.Args represents an API operation
// (not a local command like login/logout/help/completion).
func shouldInjectAPIName() bool {
	local := map[string]bool{
		"sync": true,
		"help": true, "completion": true, "version": true, "install": true,
		"list-operations": true,
	}
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return !local[arg]
	}
	// No non-flag args → show help, no injection.
	return false
}

func removeCommands(root *cobra.Command, names ...string) {
	unwanted := make(map[string]bool, len(names))
	for _, n := range names {
		unwanted[n] = true
	}
	for _, cmd := range root.Commands() {
		name := strings.SplitN(cmd.Use, " ", 2)[0]
		if unwanted[name] {
			root.RemoveCommand(cmd)
		}
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the surf CLI version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("surf " + version)
		},
	}
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Re-fetch the API spec and update local cache",
		Long:  "Force-fetches the latest OpenAPI spec from the server, updating all available commands.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			viper.Set("rsh-no-cache", true)
			if _, err := cli.Load("https://api.asksurf.ai/gateway", cli.Root); err != nil {
				return fmt.Errorf("sync failed: %w", err)
			}
			fmt.Fprintln(os.Stderr, "API spec synced.")
			return nil
		},
	}
}

func newListOperationsCmd() *cobra.Command {
	var groupByTag bool
	cmd := &cobra.Command{
		Use:   "list-operations",
		Short: "List all available API operations",
		Long:  "Show available API endpoints with methods, parameters, and descriptions.\nRun `surf sync` first if no operations appear.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			api := cli.LoadCachedAPI("surf")
			if api == nil {
				return fmt.Errorf("no cached API spec — run `surf sync` first")
			}

			if groupByTag {
				printOperationsGrouped(api.Operations)
			} else {
				printOperationsFlat(api.Operations)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&groupByTag, "group", "g", false, "Group operations by category")
	return cmd
}

func printOperationsFlat(ops []cli.Operation) {
	for _, op := range ops {
		if op.Hidden {
			continue
		}
		params := formatParams(op)
		fmt.Fprintf(os.Stdout, "  %-6s %-35s %s%s\n", op.Method, op.Name, op.Short, params)
	}
}

func printOperationsGrouped(ops []cli.Operation) {
	groups := map[string][]cli.Operation{}
	var order []string
	for _, op := range ops {
		if op.Hidden {
			continue
		}
		g := op.Group
		if g == "" {
			g = "other"
		}
		if _, seen := groups[g]; !seen {
			order = append(order, g)
		}
		groups[g] = append(groups[g], op)
	}

	for i, g := range order {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s:\n", g)
		for _, op := range groups[g] {
			params := formatParams(op)
			fmt.Fprintf(os.Stdout, "  %-6s %-35s %s%s\n", op.Method, op.Name, op.Short, params)
		}
	}
}

func formatParams(op cli.Operation) string {
	var names []string
	for _, p := range op.PathParams {
		names = append(names, "<"+p.Name+">")
	}
	for _, p := range op.QueryParams {
		names = append(names, "--"+p.Name)
	}
	if len(names) == 0 {
		return ""
	}
	return "  (" + strings.Join(names, ", ") + ")"
}
