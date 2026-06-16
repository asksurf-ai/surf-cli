package main

import (
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
