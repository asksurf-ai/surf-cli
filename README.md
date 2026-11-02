# surf

CLI for the Surf data platform — crypto market data, on-chain analytics, and more.

Every API endpoint is available as a CLI command, dynamically generated from the Surf OpenAPI spec.

## Install

```sh
curl -fsSL https://downloads.asksurf.ai/cli/releases/install.sh | sh
```

Installs to `~/.local/bin`. No sudo required.

To install a specific version:

```sh
curl -fsSL https://downloads.asksurf.ai/cli/releases/install.sh | sh -s v0.1.3
```

### Build from source

```sh
go install github.com/asksurf-ai/surf-cli/cmd/surf@latest
```

## Usage

```sh
# Save your API key
surf auth --api-key sk-xxx

# Query market data
surf market-futures --symbol BTC
surf search-project --q bitcoin

# Update available commands from latest API spec
surf sync

# Show version
surf version

# Show auth status
surf auth

# Clear saved API key
surf auth --clear
```

Run `surf help` to see all available commands.

## On-chain SQL

Every on-chain table lives in the `agent` database. Discover schema first:

```sh
surf catalog search "dex trades"
surf catalog show ethereum_dex_trades
```

Short queries run synchronously with a 30s budget:

```sh
echo '{"sql":"SELECT project, sum(amount_usd) FROM agent.ethereum_dex_trades WHERE block_date = today() - 1 GROUP BY project ORDER BY 2 DESC LIMIT 10"}' \
  | surf onchain-sql
```

Heavy queries — joins, multi-table scans, or scans whose estimated size exceeds
the synchronous budget — are rejected from the sync path with
`SYNC_QUERY_REQUIRES_ASYNC`. Run them as a durable job:

```sh
echo '{"sql":"SELECT ..."}' | surf onchain-sql-job-create
surf onchain-sql-job-get <job_id>                 # poll until terminal
surf onchain-sql-job-results <job_id> --format json
```

Or keep one call and request async inline:

```sh
echo '{"sql":"SELECT ..."}' | surf onchain-sql --prefer respond-async
```

Pass `--idempotency-key <key>` on `onchain-sql` or `onchain-sql-job-create` to
reuse an accepted job on retry instead of running the query twice. The reused
submission is still billed like any other SQL call — the key prevents duplicate
execution, not duplicate charging.

Creating the job is the billed step (same price as a sync SQL call); polling and
result download are free, and results are retained for 24 hours. `HTTP 202` means
accepted, not finished — poll `onchain-sql-job-get` until the status is terminal.

To see how a query would scan without running it, use preflight — it returns
estimated rows/bytes and the enforced budgets:

```sh
echo '{"sql":"SELECT ..."}' | surf onchain-sql-preflight
```

Preflight costs the same 4 credits as a SQL call, so reach for it when a query
keeps getting rejected rather than before every query. Check `data.accepted`:
a rejected plan still returns HTTP 200, with `accepted: false` and `warnings[]`
entries carrying a `recommended_action`.

## Authentication

API keys are resolved in this order:

1. `SURF_API_KEY` environment variable
2. OS keychain (macOS Keychain, Linux secret-service, Windows Credential Manager)
3. `~/.surf/config.json` (file fallback)

```sh
surf auth --api-key sk-xxx   # Save (prefers keychain, falls back to file)
surf auth                    # Show current key source and masked value
surf auth --clear            # Clear from both keychain and file
```

## Configuration

Configuration is stored in `~/.surf/`.

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `SURF_API_KEY` | API authentication token | — |
| `SURF_API_BASE_URL` | Override API gateway base URL | `https://api.asksurf.ai/gateway/v1` |

## Development

**Prerequisites:** Go 1.25+

### Local development

Build the binary and symlink it into `~/.local/bin/surf` so the `surf` in your
PATH resolves to your local build:

```sh
go build -o bin/surf ./cmd/surf
ln -sf "$(pwd)/bin/surf" ~/.local/bin/surf

surf version   # confirm you're on the local build
surf help
```

Rebuild after changes — the symlink keeps pointing at `bin/surf`, so a fresh
`go build -o bin/surf ./cmd/surf` is all you need to re-run against new code.

### Staging testing

To test against the staging CDN + API, install from the staging installer:

```sh
curl -fsSL https://downloads-stg.asksurf.ai/cli/releases/install.sh | sh
```

> **Do not run `surf install` afterwards.** It pulls the production binary
> from `downloads.asksurf.ai` and overwrites the staging one you just
> installed. Use `surf sync` (spec refresh only) and direct API commands
> for testing instead.

To go back to production, re-run the production installer from the
[Install](#install) section.

### Releasing

Releases are built with [GoReleaser](https://goreleaser.com/) and published to S3/CloudFront.

```sh
git tag v0.x.x
goreleaser release --clean
```

Builds binaries for Linux, macOS, and Windows (amd64/arm64) and uploads to the configured release bucket.
