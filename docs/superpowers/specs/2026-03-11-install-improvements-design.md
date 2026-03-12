# Install Improvements Design

Adopt Claude Code's install pattern: self-installing binary, bare binary distribution, XDG-compliant paths, Rosetta 2 and musl detection.

## Background

Current install flow: `install.sh` downloads a tar.gz archive, extracts it, copies the binary to `~/.surf/bin/`, and modifies shell rc files for PATH. This works but has several gaps compared to Claude Code's approach:

- No Rosetta 2 detection (downloads x64 on Apple Silicon under Rosetta)
- No musl detection for Linux
- Archives add unnecessary complexity for a single binary
- `~/.surf/bin` is non-standard and always requires PATH modification
- No self-install/self-update capability

## Design

### 1. `surf install` subcommand

New file: `cmd/surf/install.go`

**Behavior:**
1. Resolve own executable path via `os.Executable()`
2. Create `~/.local/share/surf/versions/<version>/` directory
3. Copy itself to that versioned directory
4. Create (or update) symlink: `~/.local/bin/surf` -> `~/.local/share/surf/versions/<version>/surf`
5. On Windows: copy directly to install dir (no symlinks)
6. Detect user's shell, add `~/.local/bin` to PATH in shell rc file if not already present
7. Print success message:

```
✔ surf successfully installed!

  Version: 0.2.0
  Location: ~/.local/bin/surf

  Next: Run surf --help to get started
```

**Registration:** Add to `cli.Root.AddCommand()` in `main.go` and add `"install"` to the `shouldInjectAPIName()` local commands map.

**XDG paths:**
- Binary store: `~/.local/share/surf/versions/<version>/surf`
- Symlink: `~/.local/bin/surf`
- Config (unchanged): `~/.config/surf/`

### 2. Bare binary distribution

**goreleaser.yaml changes:**
- Remove the `archives` section entirely — GoReleaser uploads bare binaries
- Keep `checksums.txt` generation
- Keep S3 blob upload config

**S3 layout:**
```
cli/releases/
├── latest
├── install.sh
├── v0.2.0/
│   ├── surf_darwin_amd64
│   ├── surf_darwin_arm64
│   ├── surf_linux_amd64
│   ├── surf_linux_arm64
│   ├── surf_windows_amd64.exe
│   ├── surf_windows_arm64.exe
│   └── checksums.txt
```

**No changes to release.yml or upload-install.yml.**

### 3. install.sh rewrite

Thin bootstrap script (~80 lines). All placement and shell integration logic moves to `surf install`.

**Flow:**
1. Detect OS (darwin, linux, windows via mingw/msys/cygwin)
2. Detect architecture (amd64, arm64)
3. Rosetta 2 detection: if darwin + x64 + `sysctl.proc_translated == 1`, switch to arm64
4. musl detection: if linux, check for musl libc (future-proofing)
5. Fetch latest version from `$CDN/latest` (or accept version as `$1`)
6. Build filename: `surf_${OS}_${ARCH}` (+ `.exe` for windows)
7. Download bare binary + `checksums.txt` to temp dir
8. Verify SHA256 checksum
9. `chmod +x`, run `./surf install`
10. Clean up temp dir

**What install.sh no longer does:** extract archives, place binary, modify shell rc files, manage PATH.

## Files changed

| File | Change |
|------|--------|
| `cmd/surf/install.go` | New — `surf install` subcommand |
| `cmd/surf/main.go` | Register install command, update local commands map |
| `.goreleaser.yaml` | Remove `archives` section |
| `install.sh` | Rewrite as thin bootstrap |
