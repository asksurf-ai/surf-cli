package openapi

import (
	"testing"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func enumNodes(vals ...string) []*yaml.Node {
	nodes := make([]*yaml.Node, len(vals))
	for i, v := range vals {
		nodes[i] = &yaml.Node{Kind: yaml.ScalarNode, Value: v}
	}
	return nodes
}

func float64Ptr(v float64) *float64 { return &v }

func TestAppendSchemaHints_Enum(t *testing.T) {
	s := &base.Schema{Enum: enumNodes("asc", "desc")}
	got := appendSchemaHints("Sort order", s)
	assert.Equal(t, "Sort order (one of: asc, desc)", got)
}

func TestAppendSchemaHints_EnumAlreadyInDesc(t *testing.T) {
	s := &base.Schema{Enum: enumNodes("market_cap", "change_24h", "volume_24h")}
	desc := "Field to sort by. market_cap sorts by total market capitalisation."
	got := appendSchemaHints(desc, s)
	assert.Equal(t, desc, got, "should not duplicate when desc already contains enum values")
}

func TestAppendSchemaHints_MaxOnly(t *testing.T) {
	s := &base.Schema{Maximum: float64Ptr(100)}
	got := appendSchemaHints("Results per page", s)
	assert.Equal(t, "Results per page (max 100)", got)
}

func TestAppendSchemaHints_MinOnly(t *testing.T) {
	s := &base.Schema{Minimum: float64Ptr(1)}
	got := appendSchemaHints("Results per page", s)
	assert.Equal(t, "Results per page (min 1)", got)
}

func TestAppendSchemaHints_MinMax(t *testing.T) {
	s := &base.Schema{Minimum: float64Ptr(1), Maximum: float64Ptr(100)}
	got := appendSchemaHints("Results per page", s)
	assert.Equal(t, "Results per page (min 1, max 100)", got)
}

func TestAppendSchemaHints_EnumAndMax(t *testing.T) {
	s := &base.Schema{
		Enum:    enumNodes("a", "b", "c"),
		Maximum: float64Ptr(50),
	}
	got := appendSchemaHints("Field", s)
	assert.Equal(t, "Field (one of: a, b, c) (max 50)", got)
}

func TestAppendSchemaHints_Empty(t *testing.T) {
	s := &base.Schema{}
	got := appendSchemaHints("Description", s)
	assert.Equal(t, "Description", got, "no enum or constraints → unchanged")
}
