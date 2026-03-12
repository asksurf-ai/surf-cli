package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	cli.AddAuth("oauth-authorization-code", &SurfAuthHandler{})

	// Customize root command.
	cli.Root.Use = "surf"
	cli.Root.Short = "Surf data platform CLI"
	cli.Root.Long = "Query the Surf data platform — crypto market data, on-chain analytics, and more."
	cli.Root.Example = "  surf market-futures --symbol BTC\n  surf search-project --q bitcoin\n  surf login"
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

	// Add custom commands directly on Root (not under the API subcommand).
	cli.Root.AddCommand(newLoginCmd())
	cli.Root.AddCommand(newLogoutCmd())
	cli.Root.AddCommand(newRefreshCmd())
	cli.Root.AddCommand(newSyncCmd())
	cli.Root.AddCommand(newVersionCmd())
	cli.Root.AddCommand(newInstallCmd())

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
		"login": true, "logout": true, "refresh": true, "sync": true,
		"help": true, "completion": true, "version": true, "install": true,
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

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to Surf via browser (OAuth)",
		Long:  "Checks for a valid session first, tries to refresh if expired, and only opens the browser as a last resort.\nTokens are cached locally in ~/.config/surf/.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := viper.GetString("rsh-profile")
			key := "surf:" + profile

			// 1. Already have a valid (non-expired) token — nothing to do.
			if expiry := cli.Cache.GetTime(key + ".expires"); !expiry.IsZero() && time.Now().Before(expiry) {
				fmt.Fprintf(os.Stderr, "Already logged in (valid until %s).\n", expiry.Local().Format("Jan 2 15:04"))
				return nil
			}

			// 2. Token expired — try silent refresh with stored refresh token.
			if rt := cli.Cache.GetString(key + ".refresh"); rt != "" {
				fmt.Fprintln(os.Stderr, "Session expired, refreshing...")
				token, err := requestToken(
					"https://surf-oauth.vercel.app/oauth/token",
					url.Values{
						"grant_type":    {"refresh_token"},
						"client_id":     {"surf-cli"},
						"refresh_token": {rt},
						"scope":         {"offline_access"},
					}.Encode(),
				)
				if err == nil {
					saveToken(key, token)
					fmt.Fprintln(os.Stderr, "Token refreshed successfully.")
					return nil
				}
				cli.LogDebug("Refresh failed: %v", err)
				fmt.Fprintln(os.Stderr, "Refresh token expired, opening browser...")
			}

			// 3. No valid session — full browser login.
			source := &surfAuthCodeTokenSource{
				ClientID:     "surf-cli",
				AuthorizeURL: "https://next.ask.surf/oauth/authorize",
				TokenURL:     "https://surf-oauth.vercel.app/oauth/token",
				Scopes:       []string{"offline_access"},
			}
			token, err := source.Token()
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			saveToken(key, token)
			fmt.Fprintln(os.Stderr, "Logged in successfully.")
			return nil
		},
	}
}

func newRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the OAuth access token",
		Long:  "Uses the stored refresh token to obtain a new access token and updates ~/.config/surf/credentials.json.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := viper.GetString("rsh-profile")
			key := "surf:" + profile
			refreshToken := cli.Cache.GetString(key + ".refresh")
			if refreshToken == "" {
				return fmt.Errorf("no refresh token found — run `surf login` first")
			}
			source := &surfRefreshTokenSource{
				ClientID:     "surf-cli",
				TokenURL:     "https://surf-oauth.vercel.app/oauth/token",
				Scopes:       []string{"offline_access"},
				RefreshToken: refreshToken,
				TokenSource: &surfAuthCodeTokenSource{
					ClientID:     "surf-cli",
					AuthorizeURL: "https://next.ask.surf/oauth/authorize",
					TokenURL:     "https://surf-oauth.vercel.app/oauth/token",
					Scopes:       []string{"offline_access"},
				},
			}
			req, _ := http.NewRequest("GET", "https://api.ask.surf/gateway/", nil)
			if err := handleToken(source, key, req); err != nil {
				return fmt.Errorf("refresh failed: %w", err)
			}
			fmt.Fprintln(os.Stderr, "Token refreshed.")
			return nil
		},
	}
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out and clear cached tokens",
		Long:  "Removes cached OAuth tokens from ~/.config/surf/. You will need to log in again.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := viper.GetString("rsh-profile")
			// Clear the entire profile auth cache — matches restish's clear-auth-cache behavior.
			cli.Cache.Set("surf:"+profile, "")
			if err := cli.Cache.WriteConfig(); err != nil {
				return fmt.Errorf("failed to clear cache: %w", err)
			}
			fmt.Fprintln(os.Stderr, "Logged out. Cached tokens cleared.")
			return nil
		},
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
			if _, err := cli.Load("https://api.ask.surf/gateway", cli.Root); err != nil {
				return fmt.Errorf("sync failed: %w", err)
			}
			fmt.Fprintln(os.Stderr, "API spec synced.")
			return nil
		},
	}
}
