package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/asksurf-ai/surf-cli/cli"
	"github.com/spf13/cobra"
)

var instantOperationBlacklist = map[string]bool{
	"onchain-sql":              true,
	"onchain-structured-query": true,
}

func newListInstantOperationsCmd() *cobra.Command {
	var groupByTag bool
	var detail bool
	var category string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "list-instant-operations",
		Aliases: []string{"list-instant-operation"},
		Short:   "List API operations available to instant mode",
		Long:    "Show OpenAPI operations available to instant mode after applying the instant blacklist.",
		Args:    cobra.NoArgs,
		Hidden:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			api := cli.LoadCachedAPI("surf")
			if api == nil {
				return fmt.Errorf("no cached API spec — run `surf sync` first")
			}

			ops := filterInstantOperations(api.Operations)
			if jsonOut {
				printInstantOperationsJSON(ops, detail, category)
			} else if groupByTag {
				printInstantOperationsGrouped(ops, detail, category)
			} else {
				printInstantOperationsFlat(ops, detail, category)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&groupByTag, "group", "g", false, "Group operations by category")
	cmd.Flags().BoolVarP(&detail, "detail", "d", false, "Show description, parameter/request schemas, and response schema for each operation")
	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category name (case-insensitive substring match)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output the instant operation contract as JSON")
	return cmd
}

func filterInstantOperations(ops []cli.Operation) []cli.Operation {
	filtered := make([]cli.Operation, 0, len(ops))
	for _, op := range ops {
		if op.Hidden || op.Deprecated != "" || instantOperationBlacklist[op.Name] {
			continue
		}
		filtered = append(filtered, op)
	}
	return filtered
}

func printInstantOperationsFlat(ops []cli.Operation, detail bool, category string) {
	ops = filterOps(ops, category)
	if detail {
		printInstantContractHeader()
	}
	for _, op := range ops {
		params := formatInstantParams(op)
		fmt.Fprintf(os.Stdout, "  %-6s %-35s %s%s\n", op.Method, op.Name, op.Short, params)
		if detail {
			printInstantOperationDetail(op)
		}
	}
}

func printInstantOperationsGrouped(ops []cli.Operation, detail bool, category string) {
	ops = filterOps(ops, category)
	if detail {
		printInstantContractHeader()
	}
	groups := map[string][]cli.Operation{}
	var order []string
	for _, op := range ops {
		g := op.Group
		if g == "" {
			g = "other"
		}
		if _, seen := groups[g]; !seen {
			order = append(order, g)
		}
		groups[g] = append(groups[g], op)
	}

	for i, g := range order {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s:\n", g)
		for _, op := range groups[g] {
			params := formatInstantParams(op)
			fmt.Fprintf(os.Stdout, "  %-6s %-35s %s%s\n", op.Method, op.Name, op.Short, params)
			if detail {
				printInstantOperationDetail(op)
			}
		}
	}
}

func formatInstantParams(op cli.Operation) string {
	var names []string
	for _, p := range op.PathParams {
		name := p.Name
		if p.Required {
			name += "*"
		}
		names = append(names, "<"+name+">")
	}
	for _, p := range op.QueryParams {
		name := "--" + p.OptionName()
		if p.Required {
			name += "*"
		}
		names = append(names, name)
	}
	for _, p := range op.HeaderParams {
		name := "--" + p.OptionName()
		if p.Required {
			name += "*"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	return "  (" + strings.Join(names, ", ") + ")"
}

func printInstantOperationDetail(op cli.Operation) {
	if desc := firstParagraph(op.Long); desc != "" {
		fmt.Fprintf(os.Stdout, "         %s\n", desc)
	}
	if details := instantDetailSections(op.Long); details != "" {
		fmt.Fprintf(os.Stdout, "         Details:\n%s\n", indentForListOperations(details, "           "))
	}
	if hints := instantOperationHints(op); len(hints) > 0 {
		fmt.Fprintf(os.Stdout, "         Instant Contract Hints:\n")
		for _, hint := range hints {
			fmt.Fprintf(os.Stdout, "           - %s\n", hint)
		}
	}
	fmt.Fprintln(os.Stdout)
}

func printInstantContractHeader() {
	fmt.Fprintln(os.Stdout, "Instant mode contract:")
	fmt.Fprintln(os.Stdout, "  - Use only operations listed by this command; it is hidden from `surf --help` but callable.")
	fmt.Fprintln(os.Stdout, "  - `*` marks required args/flags. Use `--detail`/`--json` for schemas and response fields.")
	fmt.Fprintf(os.Stdout, "  - Unavailable in instant mode: %s. Use task/research handoff for custom SQL/cohort/chain-wide aggregate work.\n\n", strings.Join(sortedInstantBlacklist(), ", "))
}

func instantDetailSections(long string) string {
	return instantSections(long, isInstantDetailHeading)
}

func instantOptionSections(long string) string {
	return instantSections(long, func(heading string) bool {
		return strings.HasPrefix(heading, "## Argument Schema") ||
			strings.HasPrefix(heading, "## Option Schema") ||
			strings.HasPrefix(heading, "## Request Schema") ||
			strings.HasPrefix(heading, "## Input Example")
	})
}

func instantResponseSections(long string) string {
	return instantSections(long, func(heading string) bool {
		return strings.HasPrefix(heading, "## Response") ||
			strings.HasPrefix(heading, "## Responses")
	})
}

func instantSections(long string, includeHeading func(string) bool) string {
	lines := strings.Split(long, "\n")
	var out []string
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if includeHeading(trimmed) {
			inSection = true
		} else if inSection && strings.HasPrefix(trimmed, "## ") {
			inSection = false
		}
		if inSection {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func isInstantDetailHeading(heading string) bool {
	return strings.HasPrefix(heading, "## Argument Schema") ||
		strings.HasPrefix(heading, "## Option Schema") ||
		strings.HasPrefix(heading, "## Input Example") ||
		strings.HasPrefix(heading, "## Request Schema") ||
		strings.HasPrefix(heading, "## Response") ||
		strings.HasPrefix(heading, "## Responses")
}

func indentForListOperations(s string, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

type instantContract struct {
	SchemaVersion  int                           `json:"schema_version"`
	Mode           string                        `json:"mode"`
	Source         string                        `json:"source"`
	Category       string                        `json:"category,omitempty"`
	OperationCount int                           `json:"operation_count"`
	Unavailable    []instantUnavailableOperation `json:"unavailable"`
	Operations     []instantContractOperation    `json:"operations"`
}

type instantUnavailableOperation struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type instantContractOperation struct {
	Name           string                 `json:"name"`
	Group          string                 `json:"group"`
	Method         string                 `json:"method"`
	Summary        string                 `json:"summary,omitempty"`
	Description    string                 `json:"description,omitempty"`
	Usage          string                 `json:"usage"`
	Params         []instantContractParam `json:"params,omitempty"`
	OptionSchema   string                 `json:"option_schema,omitempty"`
	ResponseSchema string                 `json:"response_schema,omitempty"`
	Hints          []string               `json:"hints,omitempty"`
}

type instantContractParam struct {
	Name        string   `json:"name"`
	Flag        string   `json:"flag,omitempty"`
	Location    string   `json:"location"`
	Type        string   `json:"type,omitempty"`
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Description string   `json:"description,omitempty"`
}

func printInstantOperationsJSON(ops []cli.Operation, detail bool, category string) {
	ops = filterOps(ops, category)
	contract := instantContract{
		SchemaVersion:  1,
		Mode:           "instant",
		Source:         "surf cached OpenAPI spec plus surf-cli instant blacklist",
		Category:       category,
		OperationCount: len(ops),
		Unavailable:    instantUnavailableOperations(),
		Operations:     make([]instantContractOperation, 0, len(ops)),
	}
	for _, op := range ops {
		entry := instantContractOperation{
			Name:        op.Name,
			Group:       instantGroup(op),
			Method:      op.Method,
			Summary:     op.Short,
			Description: firstParagraph(op.Long),
			Usage:       instantUsage(op),
			Params:      instantContractParams(op),
			Hints:       instantOperationHints(op),
		}
		if detail {
			entry.OptionSchema = instantOptionSections(op.Long)
			entry.ResponseSchema = instantResponseSections(op.Long)
		}
		contract.Operations = append(contract.Operations, entry)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(contract); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode instant operation contract: %v\n", err)
	}
}

func instantGroup(op cli.Operation) string {
	if op.Group == "" {
		return "other"
	}
	return op.Group
}

func instantUsage(op cli.Operation) string {
	parts := []string{"surf", op.Name}
	for _, p := range op.PathParams {
		parts = append(parts, "<"+p.Name+">")
	}
	for _, p := range append(append([]*cli.Param{}, op.QueryParams...), op.HeaderParams...) {
		if p.Required {
			parts = append(parts, "--"+p.OptionName(), "<"+p.Name+">")
		}
	}
	return strings.Join(parts, " ")
}

func instantContractParams(op cli.Operation) []instantContractParam {
	params := make([]instantContractParam, 0, len(op.PathParams)+len(op.QueryParams)+len(op.HeaderParams))
	for _, p := range op.PathParams {
		params = append(params, instantContractParam{
			Name:        p.Name,
			Location:    "path",
			Type:        p.Type,
			Required:    p.Required,
			Default:     p.Default,
			Enum:        p.Enum,
			Description: p.Description,
		})
	}
	for _, p := range op.QueryParams {
		params = append(params, instantParamContract(p, "query"))
	}
	for _, p := range op.HeaderParams {
		params = append(params, instantParamContract(p, "header"))
	}
	return params
}

func instantParamContract(p *cli.Param, location string) instantContractParam {
	return instantContractParam{
		Name:        p.Name,
		Flag:        "--" + p.OptionName(),
		Location:    location,
		Type:        p.Type,
		Required:    p.Required,
		Default:     p.Default,
		Enum:        p.Enum,
		Description: p.Description,
	}
}

func instantOperationHints(op cli.Operation) []string {
	var hints []string
	if hasParam(op, "limit") || hasParam(op, "offset") || hasParam(op, "cursor") || hasParam(op, "pagination-key") {
		hints = append(hints, "Paginated response: inspect meta.has_more/meta.total/meta.limit/meta.offset when present; continue with offset/cursor/pagination-key when the requested answer needs more rows.")
	}
	if hasParam(op, "from") || hasParam(op, "to") || hasParam(op, "time-range") || hasParam(op, "start-time") || hasParam(op, "end-time") {
		hints = append(hints, "Date/time scoped response: use explicit window flags from the user question; verify returned timestamps cover the requested period before totals or comparisons.")
	}
	switch op.Name {
	case "project-defi-metrics":
		hints = append(hints,
			"Historical DeFi metric time series: for monthly/quarterly/half-year totals or threshold checks, use explicit --from/--to and the largest --limit allowed by the option schema.",
			"Fetch fees and revenue as separate calls when both are required; do not treat a default last-20-row page as a full requested period.",
			"If the returned timestamp range does not cover the requested window, report the covered range and gap instead of a full-period total.",
		)
	case "wallet-detail":
		hints = append(hints,
			"Wallet allowance questions: run with --fields approvals, filter token + spender, and answer 0/no active allowance when no matching approval is present.",
		)
	case "onchain-tx":
		hints = append(hints,
			"Direct transaction facts: answer only fields returned for the exact --hash/--chain. Hex fields are strings; convert hex safely instead of piping 0x values into jq tonumber.",
		)
	case "search-news", "news-feed", "web-fetch", "search-web":
		hints = append(hints,
			"Exact-date news/headline questions: include the target date/entity in the query or fetched page; a current homepage is not historical evidence.",
		)
	}
	return hints
}

func hasParam(op cli.Operation, name string) bool {
	for _, p := range append(append(append([]*cli.Param{}, op.PathParams...), op.QueryParams...), op.HeaderParams...) {
		if p.OptionName() == name || strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

func instantUnavailableOperations() []instantUnavailableOperation {
	names := sortedInstantBlacklist()
	out := make([]instantUnavailableOperation, 0, len(names))
	for _, name := range names {
		out = append(out, instantUnavailableOperation{
			Name:   name,
			Reason: "blacklisted from instant mode; use task/research handoff for custom SQL, cohorts, chain-wide aggregates, or table exploration",
		})
	}
	return out
}

func sortedInstantBlacklist() []string {
	names := make([]string, 0, len(instantOperationBlacklist))
	for name := range instantOperationBlacklist {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
