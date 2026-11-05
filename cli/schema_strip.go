package cli

import (
	"sort"
	"strconv"
	"strings"
)

// HelpAfterUsageAnnotation is the command annotation that carries the
// response-format block rendered below Usage/Flags. Set by Operation.Command
// and inlined by the help template in cli.go.
const HelpAfterUsageAnnotation = "surf-help-after-usage"

// splitHelpBlocks splits an operation's Long description into the prose that
// should render above Cobra's Usage/Flags and the auto-generated response
// schemas that should render below them.
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
//   - Every response schema, success and error. Success responses come first,
//     then error codes, then the generic `default` envelope. Keeping errors
//     above meant 138 operations each opened with the same ~15-line error
//     schema before the flags, while only two operations define explicit
//     4xx/5xx responses; recovery guidance is already in the prose.
//
// Kept in `above`:
//   - `## Input Example`      — concrete example body
//   - `## Request Schema`     — body field shapes not in Cobra Flags
//   - User-authored prose (`## Rules`, `## Example`, ...)
//
// See docs/CLI_DESIGN_PRINCIPLES.md §3.5 and
// docs/CLI_V1_ALPHA_26_REVIEW.md item #5.
func splitHelpBlocks(long string) (above, below string) {
	if long == "" {
		return "", ""
	}
	var aboveLines []string
	var responses []helpResponse
	inFence := false
	dropping := false
	current := -1

	appendLine := func(line string) {
		switch {
		case dropping:
		case current >= 0:
			responses[current].lines = append(responses[current].lines, line)
		default:
			aboveLines = append(aboveLines, line)
		}
	}

	for _, raw := range strings.Split(long, "\n") {
		// Normalize CRLF so heading detection and the rendered blocks do not
		// depend on the spec file's line endings.
		line := strings.TrimSuffix(raw, "\r")

		// Track fenced code blocks so a literal `## Response` inside a
		// user-authored code example is never mistaken for a section
		// boundary. (Not strictly needed for today's spec, but cheap
		// insurance against future prose.)
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "```") {
			inFence = !inFence
			appendLine(line)
			continue
		}

		if !inFence && strings.HasPrefix(line, "## ") {
			switch {
			case isDroppedSchemaHeading(line):
				dropping, current = true, -1
				continue
			case isResponseSchemaHeading(line):
				responses = append(responses, helpResponse{heading: line, rank: responseRank(line)})
				current, dropping = len(responses)-1, false
				continue
			default:
				dropping, current = false, -1
			}
		}

		appendLine(line)
	}

	sort.SliceStable(responses, func(i, j int) bool {
		return responses[i].rank < responses[j].rank
	})

	var belowLines []string
	for _, r := range responses {
		belowLines = append(belowLines, r.heading)
		belowLines = append(belowLines, r.lines...)
	}

	return trimBlock(aboveLines), trimBlock(belowLines)
}

type helpResponse struct {
	heading string
	rank    int
	lines   []string
}

// trimBlock joins a block without the blank lines that separate it from
// neighbouring sections, so the template controls the spacing.
func trimBlock(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
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

// isResponseSchemaHeading reports whether a `## …` heading introduces a
// response schema, which always renders after Usage/Flags.
func isResponseSchemaHeading(line string) bool {
	return responseCodes(line) != nil
}

// responseCodes extracts the status codes from a `## Response …` or
// `## Responses …` heading. It returns nil for any other heading.
func responseCodes(line string) []string {
	h := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	var codes string
	switch {
	case strings.HasPrefix(h, "Responses "):
		codes = strings.TrimPrefix(h, "Responses ")
	case strings.HasPrefix(h, "Response "):
		codes = strings.TrimPrefix(h, "Response ")
	default:
		return nil
	}
	if i := strings.Index(codes, " "); i >= 0 {
		codes = codes[:i]
	}
	if codes == "" {
		return nil
	}
	return strings.Split(codes, "/")
}

// responseRank orders response blocks below the flags: success, then error
// codes, then the shared `default` envelope.
func responseRank(line string) int {
	codes := responseCodes(line)
	rank := 2
	for _, code := range codes {
		if code == "2XX" {
			return 0
		}
		n, err := strconv.Atoi(code)
		if err != nil {
			continue // `default`
		}
		switch {
		case n >= 200 && n < 300:
			return 0
		case rank > 1:
			rank = 1
		}
	}
	return rank
}
