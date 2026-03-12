# surf

CLI for the Surf data platform — crypto market data, on-chain analytics, and more.

Every API endpoint is available as a CLI command, dynamically generated from the Surf OpenAPI spec.

## Install

```sh
curl -fsSL https://agent.asksurf.ai/cli/releases/install.sh | sh
```

Installs to `~/.surf/bin`. No sudo required.

To install a specific version:

```sh
curl -fsSL https://agent.asksurf.ai/cli/releases/install.sh | sh -s v0.1.3
```

### Build from source

```sh
go install github.com/cyberconnecthq/surf-cli/cmd/surf@latest
```

## Usage

```sh
# Authenticate (opens browser)
surf login

# Query market data
surf market-futures --symbol BTC
surf search-project --q bitcoin

# Update available commands from latest API spec
surf sync

# Show version
surf version

# Log out and clear cached tokens
surf logout
```

Run `surf help` to see all available commands.

## Configuration

Config and cached tokens are stored in `~/.config/surf/`.

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
