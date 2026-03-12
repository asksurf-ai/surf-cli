# surf

CLI for the Surf data platform — crypto market data, on-chain analytics, and more.

Built as a thin wrapper around [restish](https://github.com/rest-sh/restish) that auto-loads the Surf OpenAPI spec, so every API endpoint is available as a CLI command.

## Install

```sh
sh -c "$(curl -fsSL https://agent.asksurf.ai/cli/releases/install.sh)"
```

Or install a specific version:

```sh
curl -fsSL https://agent.asksurf.ai/cli/releases/install.sh | sh -s v0.1.0
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

Run `surf help` to see all available commands. Commands are dynamically generated from the Surf API's OpenAPI spec.

## Configuration

Config and cached tokens are stored in `~/.config/surf/`.

## Development

**Prerequisites:** Go 1.25+

```sh
# Build
go build -o surf ./cmd/surf

# Build with version info
go build -ldflags "-X main.version=0.1.0" -o surf ./cmd/surf

# Run
./surf help
```

### Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions. Push a version tag to trigger a release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

This builds binaries for Linux, macOS, and Windows (amd64/arm64), uploads them to S3/CloudFront, and creates a GitHub release.
