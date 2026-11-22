# surf

CLI for the Surf data platform — crypto market data, on-chain analytics, and more.

Every API endpoint is available as a CLI command, dynamically generated from the Surf OpenAPI spec.

## Existing users: do not run `surf install` on v1.0.11 or older

surf v1.0.11 and older download updates from a host that is no longer
trusted. If you installed surf before v1.0.12, or are not sure, do not run
`surf install` or the old `surf` binary to find out. Reinstall with the
verified steps below; they replace `~/.local/bin/surf`. Releases from v1.0.12
on update from GitHub Releases and check what they download.

## Install

Binaries come from [GitHub Releases](https://github.com/asksurf-ai/surf-cli/releases).
surf installs to `~/.local/bin`; no sudo required.

> The commands below work once v1.0.12, the first release published this way,
> is out. Until then the release pages do not have these files.

### Verified install (recommended)

With the [GitHub CLI](https://cli.github.com/) (`gh`) signed in, download a
named release, check its checksum and build provenance attestation, then let
the binary install itself. Pick the release from the
[Releases page](https://github.com/asksurf-ai/surf-cli/releases) and the asset
for your platform, `surf_<linux|darwin|windows>_<amd64|arm64>[.exe]`. The
steps run in a new temporary directory and are joined with `&&`, so the binary
runs only if the download, the checksum and the attestation all pass; the
directory is removed either way:

```sh
VERSION=v1.0.12
ASSET=surf_darwin_arm64
DIR=$(mktemp -d)
( cd "$DIR" &&
  gh release download "$VERSION" -R asksurf-ai/surf-cli -p "$ASSET" -p checksums.txt &&
  awk -v f="$ASSET" '$2 == f' checksums.txt | shasum -a 256 -c - &&
  gh attestation verify "$ASSET" --repo asksurf-ai/surf-cli \
    --signer-workflow asksurf-ai/surf-cli/.github/workflows/release.yml \
    --source-ref "refs/tags/$VERSION" --deny-self-hosted-runners &&
  chmod +x "$ASSET" && ./"$ASSET" install --local )
rm -rf "$DIR"
```

On Linux without `shasum`, use `sha256sum -c -` instead.

### Install script

`install.sh` does the same for your platform. Download it from a named
release, check it, then run it (do not pipe it into `sh`). As above, the
script runs only if the download and the attestation pass:

```sh
VERSION=v1.0.12
DIR=$(mktemp -d)
( cd "$DIR" &&
  curl -fsSLO "https://github.com/asksurf-ai/surf-cli/releases/download/$VERSION/install.sh" &&
  gh attestation verify install.sh --repo asksurf-ai/surf-cli \
    --signer-workflow asksurf-ai/surf-cli/.github/workflows/release.yml \
    --source-ref "refs/tags/$VERSION" --deny-self-hosted-runners &&
  sh install.sh "$VERSION" )
rm -rf "$DIR"
```

The script checks the binary against the release's `checksums.txt`. If `gh`
is installed and signed in to github.com, it also checks the binary's build
provenance attestation and stops if that check fails for any reason. Without
a version it installs the latest release, and refuses one older than the surf
already in `~/.local/bin`. It reads that version from the directory
`~/.local/bin/surf` links to and never runs the installed binary.

Both the script and `surf install` install only v1.0.12 or newer, named or
latest. Earlier releases have no checksums or attestations and can still be
changed, so they are refused.

### Updating

`surf install` updates to the latest release and never to an older one;
`surf install v1.2.3` installs exactly that version (v1.0.12 or newer). It
checks the download against `checksums.txt` and, when `gh` is signed in to
github.com, its build provenance attestation.

To check an installed binary yourself:

```sh
gh attestation verify ~/.local/bin/surf --repo asksurf-ai/surf-cli \
  --signer-workflow asksurf-ai/surf-cli/.github/workflows/release.yml \
  --source-ref refs/tags/v1.0.12 --deny-self-hosted-runners
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

Build the CLI from the `stg` branch (to test against another API, set
`SURF_API_BASE_URL`, see [Environment Variables](#environment-variables)):

```sh
go install github.com/asksurf-ai/surf-cli/cmd/surf@stg
```

> **Do not run `surf install` afterwards.** It downloads the latest
> production release and overwrites the build you just installed. Until `stg`
> contains the GitHub Releases change, a build from `stg` still has the old
> updater, which downloads from the retired host.

To go back to production, re-run the installer from the [Install](#install)
section.

### Releasing

Releases are built with [GoReleaser](https://goreleaser.com/) by
`.github/workflows/release.yml` and published only to GitHub Releases. Releases
are immutable: once published, nothing can change their files.

1. Merge the changes to `main`.
2. Tag a `main` commit and push the tag: `git tag v1.2.3 <main-sha> && git push origin v1.2.3`.
3. Another maintainer approves the `production` environment deployment.

The workflow refuses a tag that is not `v<semver>` on `main`, then:
- runs the tests in a read-only job;
- refuses the tag if a release or draft for it already exists;
- builds the binaries for Linux, macOS and Windows (amd64/arm64) and uploads
  them, `checksums.txt` and `install.sh` to a new draft release;
- attests every file in `checksums.txt`;
- checks that the draft holds exactly those files;
- and only then publishes the release. It marks it latest only when it is the
  highest stable version, so a backport release does not move "latest" back.

If a run fails after it created the draft, delete that draft release, then
re-run the workflow.
