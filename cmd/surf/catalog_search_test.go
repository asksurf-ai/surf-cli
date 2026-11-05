package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func searchFixture() []catalogTable {
	return []catalogTable{
		{TableName: "ethereum_dex_trades", Category: "dex"},
		{TableName: "solana_dex_trades", Category: "dex"},
		{TableName: "base_dex_trades", Category: "dex"},
		{TableName: "polymarket_trades", Category: "prediction", Description: "dex-style order book"},
		{TableName: "bridge_transfers", Category: "bridge"},
	}
}

func tableNames(matches []catalogMatch) []string {
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m.table.TableName
	}
	return names
}

func TestSearchCatalogLimit(t *testing.T) {
	matches, total := searchCatalog(searchFixture(), []string{"dex"}, 2)
	assert.Equal(t, 4, total)
	// Equal scores keep catalog order, so the cut is deterministic.
	assert.Equal(t, []string{"ethereum_dex_trades", "solana_dex_trades"}, tableNames(matches))
}

func TestSearchCatalogNoLimit(t *testing.T) {
	matches, total := searchCatalog(searchFixture(), []string{"dex"}, 0)
	assert.Equal(t, 4, total)
	assert.Equal(t, []string{
		"ethereum_dex_trades", "solana_dex_trades", "base_dex_trades", "polymarket_trades",
	}, tableNames(matches))
}

func TestSearchCatalogLimitAboveTotal(t *testing.T) {
	matches, total := searchCatalog(searchFixture(), []string{"bridge"}, 10)
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"bridge_transfers"}, tableNames(matches))
}

func TestCatalogSearchAcceptsLimit(t *testing.T) {
	cmd := newCatalogSearchCmd()
	require.NotNil(t, cmd.Flags().Lookup("limit"), "catalog search should accept --limit like the API commands")
	require.NoError(t, cmd.ParseFlags([]string{"--limit", "5"}))
}

func TestCatalogSearchRejectsNegativeLimit(t *testing.T) {
	cmd := newCatalogSearchCmd()
	cmd.SetArgs([]string{"dex", "--limit", "-1"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--limit")
}
