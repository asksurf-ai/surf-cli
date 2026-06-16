package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/asksurf-ai/surf-cli/cli"
)

func TestListInstantOperationsCommandIsHidden(t *testing.T) {
	cmd := newListInstantOperationsCmd()
	if !cmd.Hidden {
		t.Fatal("list-instant-operations should be hidden from root help")
	}
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "list-instant-operation" {
		t.Fatalf("unexpected aliases: %v", cmd.Aliases)
	}
	if cmd.Flags().Lookup("json") == nil {
		t.Fatal("list-instant-operations should expose internal --json output")
	}
}

func TestFilterInstantOperationsUsesBlacklist(t *testing.T) {
	ops := []cli.Operation{
		{Name: "market-price"},
		{Name: "onchain-sql"},
		{Name: "onchain-structured-query"},
		{Name: "hidden-operation", Hidden: true},
		{Name: "deprecated-operation", Deprecated: "do not use"},
	}

	got := filterInstantOperations(ops)
	if len(got) != 1 || got[0].Name != "market-price" {
		t.Fatalf("unexpected instant operations: %#v", got)
	}
}

func TestInstantOperationHintsCaptureKnownContracts(t *testing.T) {
	op := cli.Operation{
		Name:  "project-defi-metrics",
		Group: "Project",
		QueryParams: []*cli.Param{
			{Name: "from"},
			{Name: "to"},
			{Name: "limit"},
			{Name: "offset"},
		},
	}
	got := strings.Join(instantOperationHints(op), "\n")
	for _, want := range []string{
		"Paginated response",
		"Date/time scoped response",
		"Historical DeFi metric time series",
		"default last-20-row page",
		"timestamp range",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("instantOperationHints missing %q in:\n%s", want, got)
		}
	}
}

func TestPrintInstantOperationsJSONIncludesContract(t *testing.T) {
	op := cli.Operation{
		Name:   "project-defi-metrics",
		Group:  "Project",
		Method: "GET",
		Short:  "Project DeFi Metrics",
		Long: `Historical metric.

## Option Schema:
` + "```schema" + `
{
  --q: (string)
  --limit: (integer min:1 max:100 default:20)
}
` + "```" + `

## Response 200 (application/json)

OK

` + "```schema" + `
{
  data*: [
    {
      timestamp*: (integer)
      value*: (number)
    }
  ]
}
` + "```" + `
`,
		QueryParams: []*cli.Param{
			{Name: "q", Type: "string"},
			{Name: "limit", Type: "integer", Default: 20},
		},
	}

	out := captureStdout(t, func() {
		printInstantOperationsJSON([]cli.Operation{op}, true, "")
	})

	var contract instantContract
	if err := json.Unmarshal([]byte(out), &contract); err != nil {
		t.Fatalf("invalid JSON contract: %v\n%s", err, out)
	}
	if contract.SchemaVersion != 1 || contract.Mode != "instant" {
		t.Fatalf("unexpected contract header: %#v", contract)
	}
	if len(contract.Unavailable) != 2 {
		t.Fatalf("expected blacklisted operations in JSON contract: %#v", contract.Unavailable)
	}
	if len(contract.Operations) != 1 {
		t.Fatalf("expected one operation, got %#v", contract.Operations)
	}
	got := contract.Operations[0]
	if got.Name != "project-defi-metrics" || got.Usage != "surf project-defi-metrics" {
		t.Fatalf("unexpected operation contract: %#v", got)
	}
	if len(got.Params) != 2 || got.Params[1].Flag != "--limit" || got.Params[1].Default != float64(20) {
		t.Fatalf("unexpected params: %#v", got.Params)
	}
	if !strings.Contains(got.OptionSchema, "--limit") || !strings.Contains(got.ResponseSchema, "timestamp*") {
		t.Fatalf("schemas missing expected content: %#v", got)
	}
	if !strings.Contains(strings.Join(got.Hints, "\n"), "Historical DeFi metric time series") {
		t.Fatalf("operation hints missing DeFi metric guidance: %#v", got.Hints)
	}
}

func TestFormatInstantParamsMarksRequiredInputs(t *testing.T) {
	op := cli.Operation{
		PathParams: []*cli.Param{
			{Name: "condition_id", Required: true},
		},
		QueryParams: []*cli.Param{
			{Name: "symbol", Required: true},
			{Name: "time_range"},
		},
		HeaderParams: []*cli.Param{
			{Name: "x-api-key", Required: true},
		},
	}

	got := formatInstantParams(op)
	for _, want := range []string{"<condition_id*>", "--symbol*", "--time-range", "--x-api-key*"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatInstantParams missing %q in %q", want, got)
		}
	}
}

func TestInstantDetailSectionsIncludeInputsAndResponses(t *testing.T) {
	long := `Intro paragraph.

## Option Schema:
` + "```schema" + `
{
  --symbol: (string)
}
` + "```" + `

## Notes

Do not include this section.

## Response 200 (application/json)

ok

` + "```schema" + `
{
  data*: [
    {
      price*: (number)
    }
  ]
}
` + "```" + `
`

	got := instantDetailSections(long)
	for _, want := range []string{"## Option Schema", "--symbol:", "## Response 200 (application/json)", "price*:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("instantDetailSections missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Do not include this section") {
		t.Fatalf("instantDetailSections should skip unrelated sections:\n%s", got)
	}
}
