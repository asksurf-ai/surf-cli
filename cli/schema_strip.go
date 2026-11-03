package cli

import (
	"strings"
)

// HelpAfterUsageAnnotation is the command annotation that carries the
// response-format block rendered below Usage/Flags. Set by Operation.Command
// and consumed by the "afterUsage" help-template function in cli.go.
const HelpAfterUsageAnnotation = "surf-help-after-usage"

// splitHelpBlocks splits an operation's Long description into the prose that
// should render above Cobra's Usage/Flags and the auto-generated response
// sections that should render below them.
//
// Why split instead of keep-in-place: cobra renders `.Long` before
// `.UsageString()`, so anything left in Long pushes Usage/Flags down. On the
// Surf spec the response schemas are 50–62% of --help, which buried the
// flags an agent needs to construct a call. The HelpTemplate in cli.go emits
// the returned `below` after Usage/Flags instead.
//
// Stripped from Long entirely (redundant with Cobra's rendering):
//   - `## Argument Schema:`   — duplicates positional args in Cobra Usage line
//   - `## Option Schema:`     — duplicates Cobra's `Flags:` section exactly
//
// Returned separately in `below` (rendered after Usage/Flags):
//   - `## Response <code>`    — success response field names/types
//   - `## Responses <codes>`  — merged response schemas
//
// Kept in `above`:
//   - `## Input Example`      — concrete example body
//   - `## Request Schema`     — body field shapes not in Cobra Flags
//   - `## Response default` / error responses — the error envelope is small
//     and stays where the reader is already looking
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

	return strings.TrimRight(strings.Join(aboveOut, "\n"), "\n"),
		compactResponseSchemas(strings.TrimRight(strings.Join(belowOut, "\n"), "\n"))
}

// metaFieldsKept are the response meta fields that change what an agent does
// next: whether an empty result is genuine, whether there is more to page
// through, and how many rows matched. Cached/credits/limit/offset/watermark
// are diagnostics, not decisions, and cost tokens on every --help.
var metaFieldsKept = map[string]bool{
	"empty_reason": true,
	"has_more":     true,
	"next_cursor":  true,
	"total":        true,
}

// fieldDescriptionsKept keep their doc comment in the compact view because
// they explain how to chain this endpoint to another one.
var fieldDescriptionsKept = map[string]bool{
	"id":         true,
	"project_id": true,
}

// compactResponseSchemas rewrites each ```schema fence in the response block:
// top-level keys stay, nested data object keys stay as names only, and meta is
// pruned to the decision fields. The full schema remains in --shape output and
// in the published OpenAPI spec.
func compactResponseSchemas(block string) string {
	if block == "" {
		return ""
	}
	lines := strings.Split(block, "\n")
	out := make([]string, 0, len(lines))
	inSchema := false
	scope := ""     // top-level property whose body we are inside: "data" or "meta"
	scopeIndent := 0 // indentation of that property's opening line
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inSchema = !inSchema
			scope = ""
			out = append(out, line)
			continue
		}
		if !inSchema {
			out = append(out, line)
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if scope != "" && indent <= scopeIndent {
			scope = ""
		}

		switch {
		case trimmed == "{" || trimmed == "}" || trimmed == "[" || trimmed == "]":
			out = append(out, line)
			continue
		}

		key, _, ok := parseSchemaLine(trimmed)
		if !ok {
			out = append(out, line)
			continue
		}

		if key == "$schema" {
			continue
		}

		switch {
		case scope == "":
			if key == "data" || key == "meta" {
				scope = key
				scopeIndent = indent
			}
			out = append(out, line)
		case scope == "meta":
			if metaFieldsKept[key] {
				out = append(out, line)
			}
		case scope == "data":
			if fieldDescriptionsKept[key] {
				out = append(out, line)
			} else {
				out = append(out, stripSchemaDescription(line))
			}
		}
	}
	return strings.Join(out, "\n")
}

// parseSchemaLine splits "key*: (type) desc" into its key, value and whether
// the line looks like a schema property at all.
func parseSchemaLine(trimmed string) (key, value string, ok bool) {
	colon := strings.Index(trimmed, ": ")
	if colon <= 0 {
		return "", "", false
	}
	key = strings.TrimSuffix(trimmed[:colon], "*")
	for _, r := range key {
		if !(r == '_' || r == '-' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return "", "", false
		}
	}
	return key, trimmed[colon+2:], true
}

// stripSchemaDescription drops the trailing doc comment from a schema line
// while keeping the key, required marker and type.
func stripSchemaDescription(line string) string {
	trimmed := strings.TrimSpace(line)
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	colon := strings.Index(trimmed, ": ")
	if colon <= 0 {
		return line
	}
	rest := trimmed[colon+2:]
	// Type annotations from renderSchema are parenthesised. Anything after the
	// matching closing paren is prose.
	if strings.HasPrefix(rest, "(") {
		depth := 0
		for i, r := range rest {
			switch r {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return indent + trimmed[:colon+2] + rest[:i+1]
				}
			}
		}
	}
	return line
}

func appendTo(mode int, line string, above, below *[]string) {
	switch mode {
	case 0:
		*above = append(*above, line)
	case 2:
		*below = append(*below, line)
	}
}

// stripSchemaBlocks removes auto-generated sections from an operation's
// long description that duplicate information Cobra already renders.
// It keeps the response sections in place; use splitHelpBlocks when the
// caller can render the result below Usage/Flags.
func stripSchemaBlocks(long string) string {
	above, below := splitHelpBlocks(long)
	if below == "" {
		return above
	}
	return strings.TrimRight(above+"\n\n"+below, "\n")
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
// after Usage/Flags. Only success responses move: the error envelope is small
// and better kept next to the prose that explains how to call the command.
func isBelowSchemaHeading(line string) bool {
	h := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	if strings.HasPrefix(h, "Response default") || strings.HasPrefix(h, "Responses default") {
		return false
	}
	return strings.HasPrefix(h, "Response ") || strings.HasPrefix(h, "Responses ")
}
