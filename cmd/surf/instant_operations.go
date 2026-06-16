package main

import (
	"fmt"
	"os"
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
			if groupByTag {
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
	fmt.Fprintln(os.Stdout)
}

func instantDetailSections(long string) string {
	lines := strings.Split(long, "\n")
	var out []string
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isInstantDetailHeading(trimmed) {
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
