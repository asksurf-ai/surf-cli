package cli

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/gosimple/slug"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Operation represents an API action, e.g. list-things or create-user
type Operation struct {
	Name          string   `json:"name" yaml:"name"`
	Group         string   `json:"group,omitempty" yaml:"group,omitempty"`
	Aliases       []string `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Short         string   `json:"short,omitempty" yaml:"short,omitempty"`
	Long          string   `json:"long,omitempty" yaml:"long,omitempty"`
	Method        string   `json:"method,omitempty" yaml:"method,omitempty"`
	URITemplate   string   `json:"uri_template" yaml:"uri_template"`
	PathParams    []*Param `json:"path_params,omitempty" yaml:"path_params,omitempty"`
	QueryParams   []*Param `json:"query_params,omitempty" yaml:"query_params,omitempty"`
	HeaderParams  []*Param `json:"header_params,omitempty" yaml:"header_params,omitempty"`
	BodyMediaType string   `json:"body_media_type,omitempty" yaml:"body_media_type,omitempty"`
	Examples      []string `json:"examples,omitempty" yaml:"examples,omitempty"`
	Hidden        bool     `json:"hidden,omitempty" yaml:"hidden,omitempty"`
	Deprecated    string   `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
}

// overrideServer applies the `surf-api-base-url` viper key as an override of
// uri's scheme, host, and path prefix. Returns uri unchanged if the key is
// empty or either URL fails to parse.
//
// Path handling:
//   - If override has no path (or just "/"), only scheme+host are replaced.
//   - If override has a path AND uri's path equals that path or starts with
//     it at a segment boundary, path is left alone (spec and env agree on
//     the base path).
//   - Otherwise uri's path is rewritten: the first N segments (where N is
//     the segment count of override's path) are replaced by override's path.
func overrideServer(uri string) string {
	customServer := viper.GetString("surf-api-base-url")
	if customServer == "" {
		return uri
	}
	orig, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	custom, err := url.Parse(customServer)
	if err != nil {
		return uri
	}

	orig.Scheme = custom.Scheme
	orig.Host = custom.Host

	if custom.Path != "" && custom.Path != "/" {
		customPath := strings.TrimSuffix(custom.Path, "/")
		if !strings.HasPrefix(orig.Path, customPath+"/") && orig.Path != customPath {
			customSeg := strings.Count(strings.Trim(customPath, "/"), "/") + 1
			origSegs := strings.SplitN(strings.TrimPrefix(orig.Path, "/"), "/", customSeg+1)
			if len(origSegs) > customSeg {
				orig.Path = customPath + "/" + origSegs[customSeg]
			} else {
				orig.Path = customPath
			}
		}
	}

	return orig.String()
}

// Command returns a Cobra command instance for this operation.
func (o Operation) Command() *cobra.Command {
	flags := map[string]any{}

	use := slug.Make(o.Name)
	for _, p := range o.PathParams {
		use += " " + slug.Make(p.Name)
	}

	argSpec := cobra.ExactArgs(len(o.PathParams))
	if o.BodyMediaType != "" {
		argSpec = cobra.MinimumNArgs(len(o.PathParams))
	}

	long := stripSchemaBlocks(o.Long)

	examples := ""
	for _, ex := range o.Examples {
		examples += fmt.Sprintf("  %s %s %s\n", Root.CommandPath(), use, ex)
	}

	sub := &cobra.Command{
		Use:        use,
		GroupID:    o.Group,
		Aliases:    o.Aliases,
		Short:      o.Short,
		Long:       long,
		Example:    examples,
		Args:       argSpec,
		Hidden:     o.Hidden,
		Deprecated: o.Deprecated,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentCommand = slug.Make(o.Name)
			uri := o.URITemplate

			for i, param := range o.PathParams {
				value, err := param.Parse(args[i])
				if err != nil {
					value := param.Serialize(args[i])[0]
					log.Fatalf("could not parse param %s with input %s: %v", param.Name, value, err)
				}
				// Replaces URL-encoded `{`+name+`}` in the template.
				uri = strings.Replace(uri, "{"+param.Name+"}", fmt.Sprintf("%v", value), 1)
			}

			var missing []string
			for _, param := range o.QueryParams {
				if param.Required && !cmd.Flags().Changed(param.OptionName()) {
					missing = append(missing, "--"+param.OptionName())
				}
			}
			for _, param := range o.HeaderParams {
				if param.Required && !cmd.Flags().Changed(param.OptionName()) {
					missing = append(missing, "--"+param.OptionName())
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf("missing required flag(s): %s\nSee: %s %s --help", strings.Join(missing, ", "), cmd.Root().CommandPath(), use)
			}

			query := url.Values{}
			for _, param := range o.QueryParams {
				if !cmd.Flags().Changed(param.OptionName()) {
					// This option was not passed from the shell, so there is no need to
					// send it, even if it is the default or zero value.
					continue
				}

				flag := flags[param.Name]
				for _, v := range param.Serialize(flag) {
					query.Add(param.Name, v)
				}
			}
			queryEncoded := query.Encode()
			if queryEncoded != "" {
				if strings.Contains(uri, "?") {
					uri += "&"
				} else {
					uri += "?"
				}
				uri += queryEncoded
			}

			uri = overrideServer(uri)

			headers := http.Header{}
			for _, param := range o.HeaderParams {
				if !cmd.Flags().Changed(param.OptionName()) {
					// This option was not passed from the shell, so there is no need to
					// send it, even if it is the default or zero value.
					continue
				}

				for _, v := range param.Serialize(flags[param.Name]) {
					headers.Add(param.Name, v)
				}
			}

			var body io.Reader

			if o.BodyMediaType != "" {
				b, err := GetBody(o.BodyMediaType, args[len(o.PathParams):])
				if err != nil {
					panic(err)
				}
				body = strings.NewReader(b)
			}

			req, _ := http.NewRequest(o.Method, uri, body)
			req.Header = headers
			MakeRequestAndFormat(req)
			return nil
		},
	}

	for _, p := range o.QueryParams {
		flags[p.Name] = p.AddFlag(sub.Flags())
	}

	for _, p := range o.HeaderParams {
		flags[p.Name] = p.AddFlag(sub.Flags())
	}

	sub.Flags().SetNormalizeFunc(NormalizeSnakeCaseFlags)
	sub.SetFlagErrorFunc(flagErrorFuncWithSuggestions)

	return sub
}

// flagErrorFuncWithSuggestions intercepts pflag parse errors and, for unknown
// flags, appends a "did you mean?" hint with the closest valid flags on the
// same command. Common agent guesses like --query (→ --q), --username (→
// --handle), --time-range (on an interval-only endpoint) will match their
// intended flag via edit distance, so the agent sees the right answer without
// having to rerun --help.
func flagErrorFuncWithSuggestions(cmd *cobra.Command, err error) error {
	msg := err.Error()
	if !strings.HasPrefix(msg, "unknown flag:") && !strings.HasPrefix(msg, "unknown shorthand flag:") {
		return err
	}
	bad := extractUnknownFlagName(msg)
	if bad == "" {
		return err
	}
	suggestions := suggestFlagNames(cmd, bad)
	if len(suggestions) == 0 {
		return err
	}
	hint := "\n\nDid you mean one of these?\n"
	for _, s := range suggestions {
		hint += "\t--" + s + "\n"
	}
	hint += "\nRun '" + cmd.Root().Name() + " " + cmd.Name() + " --help' for all flags."
	return fmt.Errorf("%s%s", msg, hint)
}

// extractUnknownFlagName pulls the flag name out of pflag's error strings,
// e.g. `unknown flag: --query` → `query`.
func extractUnknownFlagName(msg string) string {
	for _, prefix := range []string{"unknown flag: --", "unknown flag: -", "unknown shorthand flag: "} {
		if strings.HasPrefix(msg, prefix) {
			rest := strings.TrimPrefix(msg, prefix)
			// Truncate at first whitespace or newline.
			if i := strings.IndexAny(rest, " \t\n"); i >= 0 {
				rest = rest[:i]
			}
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// suggestFlagNames returns up to 3 valid flag names from cmd whose Levenshtein
// distance to bad is ≤ 3. Shorter valid names get priority on ties so that a
// 1-char guess like --query doesn't miss --q.
func suggestFlagNames(cmd *cobra.Command, bad string) []string {
	type candidate struct {
		name string
		dist int
	}
	var cands []candidate
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		d := levenshtein(bad, f.Name)
		// Allow up to 3 edits, OR if the bad name contains the real flag name
		// as a substring (covers --query → --q, --keyword → --q harder but
		// at least --timerange → --time-range).
		if d <= 3 || strings.Contains(bad, f.Name) || strings.Contains(f.Name, bad) {
			cands = append(cands, candidate{f.Name, d})
		}
	})
	// Sort by distance asc, then by length asc (so short names like "q" win
	// ties against longer partial matches), then alphabetically.
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		if len(cands[i].name) != len(cands[j].name) {
			return len(cands[i].name) < len(cands[j].name)
		}
		return cands[i].name < cands[j].name
	})
	if len(cands) > 3 {
		cands = cands[:3]
	}
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.name)
	}
	return out
}

// levenshtein computes the edit distance between a and b. Classic DP; fine
// for short flag-name inputs.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = del
			if ins < curr[j] {
				curr[j] = ins
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

// NormalizeSnakeCaseFlags silently converts underscore-separated flag names
// to kebab-case. This allows --time_range to work as --time-range.
// No warning is printed because pflag calls this function during flag
// registration (not just user input), which would produce false warnings.
func NormalizeSnakeCaseFlags(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
	}
	return pflag.NormalizedName(name)
}
