package cli

import "strings"

// stripSchemaBlocks removes auto-generated sections from an operation's
// long description that duplicate information Cobra already renders.
//
// Stripped (redundant — Cobra shows the same info under Flags:/Usage):
//   - `## Argument Schema:`   — duplicates positional args in Cobra Usage line
//   - `## Option Schema:`     — duplicates Cobra's `Flags:` section exactly
//
// Kept (unique information not available elsewhere in --help):
//   - `## Response <code>`    — response field names, types, required markers
//   - `## Responses <codes>`  — merged response schemas
//   - `## Input Example`      — concrete example body
//   - `## Request Schema`     — body field shapes not in Cobra Flags
//   - User-authored prose (`## Rules`, `## Example`, ...)
//
// See docs/CLI_DESIGN_PRINCIPLES.md §3.5 and
// docs/CLI_V1_ALPHA_26_REVIEW.md item #5.
func stripSchemaBlocks(long string) string {
	if long == "" {
		return ""
	}
	lines := strings.Split(long, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	dropping := false
	for _, line := range lines {
		// Track fenced code blocks so a literal `## Response` inside a
		// user-authored code example is never mistaken for a section
		// boundary. (Not strictly needed for today's spec, but cheap
		// insurance against future prose.)
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "```") {
			inFence = !inFence
			if !dropping {
				out = append(out, line)
			}
			continue
		}

		if !inFence && strings.HasPrefix(line, "## ") {
			if isDroppedSchemaHeading(line) {
				dropping = true
				continue
			}
			dropping = false
		}

		if !dropping {
			out = append(out, line)
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// isDroppedSchemaHeading reports whether a `## …` heading introduces one
// of the auto-generated schema dumps we want to suppress from --help.
//
// Keep in sync with the explicit `desc += "\n## …"` statements in
// openapi/openapi.go that emit these headings.
func isDroppedSchemaHeading(line string) bool {
	h := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	switch {
	case strings.HasPrefix(h, "Argument Schema"),
		strings.HasPrefix(h, "Option Schema"):
		return true
	}
	return false
}
