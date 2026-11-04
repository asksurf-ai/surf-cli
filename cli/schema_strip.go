package cli

import (
	"strconv"
	"strings"
)

// HelpAfterUsageAnnotation is the command annotation that carries the
// response-format block rendered below Usage/Flags. Set by Operation.Command
// and inlined by the help template in cli.go.
const HelpAfterUsageAnnotation = "surf-help-after-usage"

// splitHelpBlocks splits an operation's Long description into the prose that
// should render above Cobra's Usage/Flags and the auto-generated success
// responses that should render below them.
//
// Cobra renders .Long before .UsageString(), so anything left in Long pushes
// Usage/Flags down. On the Surf spec the response schemas are 50–62% of
// --help, which buried the flags an agent needs to construct a call. The help
// template in cli.go emits the returned `below` after Usage/Flags instead.
//
// Stripped from Long entirely (redundant with Cobra's rendering):
//   - `## Argument Schema:`   — duplicates positional args in Cobra Usage line
//   - `## Option Schema:`     — duplicates Cobra's `Flags:` section exactly
//
// Returned separately in `below` (rendered after Usage/Flags):
//   - `## Response 2xx`       — success response field names/types
//   - `## Responses 2xx`      — merged success schemas
//
// Kept in `above`:
//   - `## Input Example`      — concrete example body
//   - `## Request Schema`     — body field shapes not in Cobra Flags
//   - Every error response (`default`, 4xx, 5xx) — recovery guidance such as
//     `recommended_action: use_async_job` belongs next to the call, not after
//     the flags
//   - User-authored prose (`## Rules`, `## Example`, ...)
//
// See docs/CLI_DESIGN_PRINCIPLES.md §3.5 and
// docs/CLI_V1_ALPHA_26_REVIEW.md item #5.
func splitHelpBlocks(long string) (above, below string) {
	if long == "" {
		return "", ""
	}
	lines := strings.Split(long, "\n")
	var aboveOut, belowOut []string
	inFence := false
	mode := 0 // 0 = above, 1 = dropping, 2 = below

	for _, line := range lines {
		// Normalize CRLF so heading detection and the rendered blocks do not
		// depend on the spec file's line endings.
		line = strings.TrimSuffix(line, "\r")

		// Track fenced code blocks so a literal `## Response` inside a
		// user-authored code example is never mistaken for a section
		// boundary. (Not strictly needed for today's spec, but cheap
		// insurance against future prose.)
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "```") {
			inFence = !inFence
			appendTo(mode, line, &aboveOut, &belowOut)
			continue
		}

		if !inFence && strings.HasPrefix(line, "## ") {
			switch {
			case isDroppedSchemaHeading(line):
				mode = 1
				continue
			case isBelowSchemaHeading(line):
				mode = 2
			default:
				mode = 0
			}
		}

		appendTo(mode, line, &aboveOut, &belowOut)
	}

	return strings.TrimRight(strings.Join(aboveOut, "\n"), "\r\n"),
		strings.TrimRight(strings.Join(belowOut, "\n"), "\r\n")
}

func appendTo(mode int, line string, above, below *[]string) {
	switch mode {
	case 0:
		*above = append(*above, line)
	case 2:
		*below = append(*below, line)
	}
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

// isBelowSchemaHeading reports whether a `## …` heading should be rendered
// after Usage/Flags. Only 2xx responses move: error responses carry recovery
// guidance and stay next to the prose that explains how to call the command.
func isBelowSchemaHeading(line string) bool {
	h := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	var codes string
	switch {
	case strings.HasPrefix(h, "Responses "):
		codes = strings.TrimPrefix(h, "Responses ")
	case strings.HasPrefix(h, "Response "):
		codes = strings.TrimPrefix(h, "Response ")
	default:
		return false
	}
	if i := strings.Index(codes, " "); i >= 0 {
		codes = codes[:i]
	}
	for _, code := range strings.Split(codes, "/") {
		if code == "2XX" {
			continue
		}
		n, err := strconv.Atoi(code)
		if err != nil || n < 200 || n >= 300 {
			return false
		}
	}
	return true
}
