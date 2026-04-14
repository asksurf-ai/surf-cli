# surf

CLI for the Surf data platform — crypto market data, on-chain analytics, and more.

Every API endpoint is available as a CLI command, dynamically generated from the Surf OpenAPI spec.

## Install

```sh
curl -fsSL https://downloads.asksurf.ai/cli/releases/install.sh | sh
```

Installs to `~/.surf/bin`. No sudo required.

To install a specific version:

```sh
curl -fsSL https://downloads.asksurf.ai/cli/releases/install.sh | sh -s v0.1.3
```

### Build from source

```sh
go install github.com/cyberconnecthq/surf-cli/cmd/surf@latest
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

```sh
# Build
go build -o surf ./cmd/surf

# Run
./surf help
```

### Releasing

Releases are built with [GoReleaser](https://goreleaser.com/) and published to S3/CloudFront.

```sh
git tag v0.x.x
goreleaser release --clean
```

Builds binaries for Linux, macOS, and Windows (amd64/arm64) and uploads to `s3://surf-cli-releases/cli/releases/<tag>/`.
