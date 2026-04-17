package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const catalogURL = "https://metadata.asksurf.ai/data-catalog/swell_data_catalog.json"

// --- JSON structures (subset of swell_data_catalog.json) ---

type catalog struct {
	GeneratedAt    string            `json:"generated_at"`
	TotalTables    int               `json:"total_tables"`
	Connection     json.RawMessage   `json:"connection"`
	BestPractices  []string          `json:"query_best_practices"`
	EntityLinking  []string          `json:"entity_linking"`
	JoinPatterns   []joinPattern     `json:"join_patterns"`
	Categories     []catalogCategory `json:"categories"`
	Tables         []catalogTable    `json:"tables"`
}

type joinPattern struct {
	Table   string `json:"table"`
	JoinKey string `json:"join_key"`
	UseWith string `json:"use_with"`
	Pattern string `json:"pattern"`
	Note    string `json:"note"`
}

type catalogCategory struct {
	Name       string   `json:"name"`
	TableCount int      `json:"table_count"`
	Tables     []string `json:"tables"`
}

type catalogTable struct {
	TableName       string            `json:"table_name"`
	Database        string            `json:"database"`
	Category        string            `json:"category"`
	Chain           string            `json:"chain"`
	Description     string            `json:"description"`
	WhenToUse       string            `json:"when_to_use"`
	WhenNotToUse    string            `json:"when_not_to_use"`
	RowCount        int64             `json:"row_count"`
	SizeClass       string            `json:"size_class"`
	Engine          string            `json:"engine"`
	OrderingKey     string            `json:"ordering_key"`
	OrderingKeyNote string            `json:"ordering_key_note"`
	PartitionKey    string            `json:"partition_key"`
	Freshness       string            `json:"freshness"`
	QueryTips       []string          `json:"query_tips"`
	Limitations     []string          `json:"limitations"`
	RelatedTables   []string          `json:"related_tables"`
	SkipIndices     []skipIndex       `json:"skip_indices"`
	Columns         []catalogColumn   `json:"columns"`
	SampleQueries   []sampleQuery     `json:"sample_queries"`
	CustomView      bool              `json:"custom_view,omitempty"`
	ViewSQL         string            `json:"view_sql,omitempty"`
}

type skipIndex struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Expression  string `json:"expression"`
	Granularity int    `json:"granularity"`
}

type catalogColumn struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	IsPartitionKey bool   `json:"is_partition_key"`
	IsSortingKey   bool   `json:"is_sorting_key"`
}

type sampleQuery struct {
	Title string `json:"title"`
	SQL   string `json:"sql"`
}

// --- Fetch ---

func fetchCatalog() (*catalog, error) {
	resp, err := http.Get(catalogURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("catalog returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog: %w", err)
	}

	var cat catalog
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, fmt.Errorf("failed to parse catalog: %w", err)
	}
	return &cat, nil
}

// --- Commands ---

func newCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Query the Surf data catalog",
		Long:  "Browse tables, schemas, and query optimization tips for the Surf ClickHouse data platform.",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.AddCommand(newCatalogListCmd())
	cmd.AddCommand(newCatalogShowCmd())
	cmd.AddCommand(newCatalogSearchCmd())
	cmd.AddCommand(newCatalogPracticesCmd())

	return cmd
}

func newCatalogListCmd() *cobra.Command {
	var category string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all tables in the data catalog",
		Long:  "Display all agent-accessible tables grouped by category with row counts and size class.",
		Example: `  surf catalog list
  surf catalog list --category "DEX Trades"
  surf catalog list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := fetchCatalog()
			if err != nil {
				return err
			}

			tables := cat.Tables
			if category != "" {
				lc := strings.ToLower(category)
				var filtered []catalogTable
				for _, t := range tables {
					tc := strings.ToLower(t.Category)
					if tc == lc || strings.Contains(tc, lc) {
						filtered = append(filtered, t)
					}
				}
				if len(filtered) == 0 {
					// Suggest closest category
					var cats []string
					seen := map[string]bool{}
					for _, t := range tables {
						if !seen[t.Category] {
							seen[t.Category] = true
							cats = append(cats, t.Category)
						}
					}
					fmt.Fprintf(os.Stderr, "No category matching %q. Available: %s\n", category, strings.Join(cats, ", "))
					return nil
				}
				tables = filtered
			}

			if jsonOut {
				type listEntry struct {
					Table     string `json:"table"`
					Category  string `json:"category"`
					SizeClass string `json:"size_class"`
					RowCount  int64  `json:"row_count"`
				}
				entries := make([]listEntry, len(tables))
				for i, t := range tables {
					entries[i] = listEntry{t.TableName, t.Category, t.SizeClass, t.RowCount}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			// Group by category
			type group struct {
				name   string
				tables []catalogTable
			}
			groupMap := map[string]*group{}
			var order []string
			for i := range tables {
				t := &tables[i]
				g, ok := groupMap[t.Category]
				if !ok {
					g = &group{name: t.Category}
					groupMap[t.Category] = g
					order = append(order, t.Category)
				}
				g.tables = append(g.tables, *t)
			}

			fmt.Fprintf(os.Stdout, "%d tables (updated %s)\n\n", len(tables), cat.GeneratedAt)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, catName := range order {
				g := groupMap[catName]
				fmt.Fprintf(w, "%s (%d)\n", g.name, len(g.tables))
				for _, t := range g.tables {
					fmt.Fprintf(w, "  %s\t%s\t%s\n", t.TableName, t.SizeClass, formatRowCount(t.RowCount))
				}
				fmt.Fprintln(w)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category name")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func newCatalogShowCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show <table>",
		Short: "Show full details for a table",
		Long:  "Display schema, ordering key, skip indices, query tips, and sample queries for a table.",
		Example: `  surf catalog show ethereum_dex_trades
  surf catalog show agent.polymarket_trades
  surf catalog show polymarket_trades --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := fetchCatalog()
			if err != nil {
				return err
			}

			table := findTable(cat, args[0])
			if table == nil {
				return fmt.Errorf("table %q not found — run `surf catalog list` to see available tables", args[0])
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(table)
			}

			printTableDetail(table)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func newCatalogSearchCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search tables by keyword",
		Long:  "Search table names, descriptions, categories, and column names for matching terms.",
		Example: `  surf catalog search "polymarket volume"
  surf catalog search bridge
  surf catalog search tvl --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := fetchCatalog()
			if err != nil {
				return err
			}

			query := strings.ToLower(args[0])
			terms := strings.Fields(query)

			type match struct {
				table  catalogTable
				score  int
				reason string
			}

			var matches []match
			for _, t := range cat.Tables {
				score, reason := scoreMatch(t, terms)
				if score > 0 {
					matches = append(matches, match{t, score, reason})
				}
			}

			sort.Slice(matches, func(i, j int) bool {
				return matches[i].score > matches[j].score
			})

			if len(matches) == 0 {
				fmt.Fprintf(os.Stderr, "No tables matching %q\n", args[0])
				return nil
			}

			if jsonOut {
				type result struct {
					Table       string `json:"table"`
					Category    string `json:"category"`
					Description string `json:"description"`
					Match       string `json:"match_reason"`
				}
				results := make([]result, len(matches))
				for i, m := range matches {
					results[i] = result{m.table.TableName, m.table.Category, m.table.Description, m.reason}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			fmt.Fprintf(os.Stdout, "%d tables matching %q\n\n", len(matches), args[0])
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "TABLE\tCATEGORY\tMATCH\n")
			for _, m := range matches {
				fmt.Fprintf(w, "%s\t%s\t%s\n", m.table.TableName, m.table.Category, m.reason)
			}
			return w.Flush()
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func newCatalogPracticesCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:     "practices",
		Short:   "Show ClickHouse query best practices",
		Long:    "Display query optimization rules, entity linking patterns, and connection settings.",
		Example: "  surf catalog practices",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := fetchCatalog()
			if err != nil {
				return err
			}

			if jsonOut {
				out := map[string]any{
					"query_best_practices": cat.BestPractices,
					"entity_linking":       cat.EntityLinking,
					"join_patterns":        cat.JoinPatterns,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			fmt.Println("Query Best Practices")
			fmt.Println(strings.Repeat("─", 50))
			for i, p := range cat.BestPractices {
				fmt.Fprintf(os.Stdout, "  %2d. %s\n", i+1, p)
			}

			fmt.Println()
			fmt.Println("Entity Linking")
			fmt.Println(strings.Repeat("─", 50))
			for _, e := range cat.EntityLinking {
				fmt.Fprintf(os.Stdout, "  • %s\n", e)
			}

			if len(cat.JoinPatterns) > 0 {
				fmt.Println()
				fmt.Println("JOIN Patterns")
				fmt.Println(strings.Repeat("─", 50))
				for _, jp := range cat.JoinPatterns {
					fmt.Printf("  %s (key: %s)\n", jp.Table, jp.JoinKey)
					fmt.Printf("    %s\n", jp.Pattern)
					fmt.Printf("    %s\n\n", jp.Note)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

// --- Helpers ---

func findTable(cat *catalog, name string) *catalogTable {
	name = strings.ToLower(name)
	// Strip "agent." or "curated." prefix if user provided it
	short := name
	short = strings.TrimPrefix(short, "agent.")
	short = strings.TrimPrefix(short, "curated.")

	for i := range cat.Tables {
		t := &cat.Tables[i]
		tLower := strings.ToLower(t.TableName)
		if tLower == name || tLower == "agent."+short || tLower == "curated."+short {
			return t
		}
	}
	return nil
}

func scoreMatch(t catalogTable, terms []string) (int, string) {
	score := 0
	var reasons []string

	nameLower := strings.ToLower(t.TableName)
	descLower := strings.ToLower(t.Description)
	catLower := strings.ToLower(t.Category)
	useLower := strings.ToLower(t.WhenToUse)

	for _, term := range terms {
		if strings.Contains(nameLower, term) {
			score += 10
			reasons = append(reasons, "name")
		}
		if strings.Contains(catLower, term) {
			score += 5
			reasons = append(reasons, "category")
		}
		if strings.Contains(descLower, term) {
			score += 3
			reasons = append(reasons, "description")
		}
		if strings.Contains(useLower, term) {
			score += 2
			reasons = append(reasons, "use_case")
		}
		// Search column names
		for _, c := range t.Columns {
			if strings.Contains(strings.ToLower(c.Name), term) {
				score += 1
				reasons = append(reasons, "column:"+c.Name)
				break
			}
		}
		// Search related tables
		for _, rt := range t.RelatedTables {
			if strings.Contains(strings.ToLower(rt), term) {
				score += 1
				reasons = append(reasons, "related:"+rt)
				break
			}
		}
	}

	// Deduplicate reasons
	seen := map[string]bool{}
	var unique []string
	for _, r := range reasons {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}

	return score, strings.Join(unique, ", ")
}

func formatRowCount(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB rows", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM rows", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK rows", float64(n)/1e3)
	case n > 0:
		return fmt.Sprintf("%d rows", n)
	default:
		return ""
	}
}

func printTableDetail(t *catalogTable) {
	fmt.Printf("%s\n", t.TableName)
	fmt.Println(strings.Repeat("═", len(t.TableName)))
	fmt.Println()

	if t.Description != "" {
		fmt.Println(t.Description)
		fmt.Println()
	}

	// Metadata
	fmt.Printf("Category:      %s\n", t.Category)
	if t.Chain != "" {
		fmt.Printf("Chain:         %s\n", t.Chain)
	}
	fmt.Printf("Size:          %s (%s)\n", t.SizeClass, formatRowCount(t.RowCount))
	fmt.Printf("Engine:        %s\n", t.Engine)
	if t.OrderingKey != "" {
		fmt.Printf("ORDER BY:      %s\n", t.OrderingKey)
	}
	if t.PartitionKey != "" {
		fmt.Printf("PARTITION BY:  %s\n", t.PartitionKey)
	}
	fmt.Printf("Freshness:     %s\n", t.Freshness)

	if t.OrderingKeyNote != "" {
		fmt.Printf("\n  ℹ  %s\n", t.OrderingKeyNote)
	}

	// Skip indices
	if len(t.SkipIndices) > 0 {
		fmt.Println()
		fmt.Println("Skip Indices (bloom filters for fast lookups)")
		for _, idx := range t.SkipIndices {
			fmt.Printf("  • %s: %s on %s\n", idx.Name, idx.Type, idx.Expression)
		}
	}

	// Columns
	if len(t.Columns) > 0 {
		fmt.Println()
		fmt.Println("Columns")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, c := range t.Columns {
			flags := ""
			if c.IsSortingKey {
				flags = " [sort]"
			}
			if c.IsPartitionKey {
				flags = " [partition]"
			}
			desc := ""
			if c.Description != "" {
				desc = " — " + c.Description
			}
			fmt.Fprintf(w, "  %s\t%s%s%s\n", c.Name, c.Type, flags, desc)
		}
		w.Flush()
	}

	// View SQL for custom views
	if t.ViewSQL != "" {
		fmt.Println()
		fmt.Println("View SQL")
		for _, line := range strings.Split(t.ViewSQL, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	// When to use
	if t.WhenToUse != "" {
		fmt.Println()
		fmt.Printf("When to use:     %s\n", t.WhenToUse)
	}
	if t.WhenNotToUse != "" {
		fmt.Printf("When NOT to use: %s\n", t.WhenNotToUse)
	}

	// Query tips
	if len(t.QueryTips) > 0 {
		fmt.Println()
		fmt.Println("Query Tips")
		for _, tip := range t.QueryTips {
			fmt.Printf("  • %s\n", tip)
		}
	}

	// Related tables
	if len(t.RelatedTables) > 0 {
		fmt.Println()
		fmt.Printf("Related: %s\n", strings.Join(t.RelatedTables, ", "))
	}

	// Sample queries
	if len(t.SampleQueries) > 0 {
		fmt.Println()
		fmt.Println("Sample Queries")
		for _, sq := range t.SampleQueries {
			fmt.Printf("\n  -- %s\n", sq.Title)
			for _, line := range strings.Split(sq.SQL, "\n") {
				fmt.Printf("  %s\n", line)
			}
		}
	}
}
