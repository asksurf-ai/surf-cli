# Agent Entry

Read `README.md` before changing the CLI.

## Hard rules

- The Go CLI is the public entrypoint for Surf data and on-chain SQL.
- Generate commands from the released OpenAPI contract; do not depend on an unreleased staging schema.
- Keep authentication, command discovery, streaming, and error output test-covered.
- Verify install and downloaded-binary behavior for release changes.
- Do not print API keys or credentials.
- Do not merge, tag, or publish from an agent task without explicit approval.

