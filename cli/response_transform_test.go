package cli

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformResponseForCommandAddsOnchainTxDecimalFields(t *testing.T) {
	reset(false)

	body := map[string]any{
		"$schema": "https://api.asksurf.ai/schemas/TransactionResponse.json",
		"data": []any{
			map[string]any{
				"hash":        "0xabc",
				"blockNumber": "0x157f411",
				"gas":         "0x5208",
				"gasPrice":    "0x3b9aca00",
				"value":       "0xde0b6b3a7640000",
			},
		},
	}

	transformed, err := transformResponseForCommand("onchain-tx", body)
	require.NoError(t, err)
	got := transformed.(map[string]any)
	tx := got["data"].([]any)[0].(map[string]any)

	assert.Equal(t, "0x157f411", tx["blockNumber"])
	assert.Equal(t, "22541329", tx["blockNumberDecimal"])
	assert.Equal(t, "21000", tx["gasDecimal"])
	assert.Equal(t, "1000000000", tx["gasPriceDecimal"])
	assert.Equal(t, "1000000000000000000", tx["valueDecimal"])
	assert.Equal(t, "1", tx["valueNativeDecimal"])
}

func TestTransformResponseForCommandAddsQ188BlockDecimal(t *testing.T) {
	reset(false)

	body := map[string]any{
		"data": []any{
			map[string]any{
				"blockNumber": "0x1630d13",
				"value":       "0x0",
			},
		},
	}

	transformed, err := transformResponseForCommand("onchain-tx", body)
	require.NoError(t, err)
	got := transformed.(map[string]any)
	tx := got["data"].([]any)[0].(map[string]any)

	assert.Equal(t, "23268627", tx["blockNumberDecimal"])
	assert.Equal(t, "0", tx["valueDecimal"])
	assert.Equal(t, "0", tx["valueNativeDecimal"])
}

func TestTransformResponseForCommandHandlesFractionalNativeValue(t *testing.T) {
	reset(false)

	body := map[string]any{
		"data": []any{
			map[string]any{
				"value": "0x2386f26fc10000", // 0.01 native units at 18 decimals.
			},
		},
	}

	transformed, err := transformResponseForCommand("onchain-tx", body)
	require.NoError(t, err)
	got := transformed.(map[string]any)
	tx := got["data"].([]any)[0].(map[string]any)

	assert.Equal(t, "10000000000000000", tx["valueDecimal"])
	assert.Equal(t, "0.01", tx["valueNativeDecimal"])
}

func TestTransformResponseForCommandDoesNotOverrideExistingFields(t *testing.T) {
	reset(false)

	body := map[string]any{
		"data": []any{
			map[string]any{
				"blockNumber":        "0x10",
				"blockNumberDecimal": "existing",
				"value":              "0x0",
				"valueNativeDecimal": "existing-native",
			},
		},
	}

	transformed, err := transformResponseForCommand("onchain-tx", body)
	require.NoError(t, err)
	got := transformed.(map[string]any)
	tx := got["data"].([]any)[0].(map[string]any)

	assert.Equal(t, "existing", tx["blockNumberDecimal"])
	assert.Equal(t, "0", tx["valueDecimal"])
	assert.Equal(t, "existing-native", tx["valueNativeDecimal"])
}

func TestTransformResponseForCommandIgnoresOtherCommands(t *testing.T) {
	reset(false)

	body := map[string]any{
		"data": []any{
			map[string]any{
				"blockNumber": "0x10",
			},
		},
	}

	transformed, err := transformResponseForCommand("market-price", body)
	require.NoError(t, err)
	got := transformed.(map[string]any)
	tx := got["data"].([]any)[0].(map[string]any)

	assert.NotContains(t, tx, "blockNumberDecimal")
}

func TestTransformResponseForCommandSearchWebAgentView(t *testing.T) {
	reset(false)
	viper.Set("rsh-agent-view", "results")

	body := map[string]any{
		"data": []any{
			map[string]any{
				"title":       "Aave governance update",
				"description": "Aave published an update.",
				"url":         "https://example.com/aave",
				"content":     "large markdown that should not survive",
			},
			map[string]any{
				"name":    "Fallback title",
				"snippet": "Fallback snippet",
				"link":    "https://example.com/fallback",
			},
		},
	}

	transformed, err := transformResponseForCommand("search-web", body)
	require.NoError(t, err)

	assert.Equal(t, []any{
		map[string]any{
			"title":       "Aave governance update",
			"description": "Aave published an update.",
			"url":         "https://example.com/aave",
		},
		map[string]any{
			"title":       "Fallback title",
			"description": "Fallback snippet",
			"url":         "https://example.com/fallback",
		},
	}, transformed)
}

func TestTransformResponseForCommandSearchWebAgentViewHandlesLegacyResultsShape(t *testing.T) {
	reset(false)
	viper.Set("rsh-agent-view", "results")

	body := map[string]any{
		"data": map[string]any{
			"results": []any{
				map[string]any{
					"title":       "Legacy result",
					"description": "Legacy shape",
					"url":         "https://example.com/legacy",
				},
			},
		},
	}

	transformed, err := transformResponseForCommand("search-web", body)
	require.NoError(t, err)

	assert.Equal(t, []any{
		map[string]any{
			"title":       "Legacy result",
			"description": "Legacy shape",
			"url":         "https://example.com/legacy",
		},
	}, transformed)
}

func TestTransformResponseForCommandProjectContractsAgentView(t *testing.T) {
	reset(false)
	viper.Set("rsh-agent-view", "contracts")

	body := map[string]any{
		"data": map[string]any{
			"contracts": map[string]any{
				"contracts": []any{
					map[string]any{
						"chain":            "ethereum",
						"contract_address": "0xabc",
						"symbol":           "ABC",
						"name":             "Example Token",
						"decimals":         float64(18),
						"verbose":          "dropped",
					},
				},
			},
		},
	}

	transformed, err := transformResponseForCommand("project-detail", body)
	require.NoError(t, err)

	assert.Equal(t, []any{
		map[string]any{
			"chain":    "ethereum",
			"address":  "0xabc",
			"symbol":   "ABC",
			"name":     "Example Token",
			"decimals": float64(18),
		},
	}, transformed)
}

func TestTransformResponseForCommandMarketTGESummaryAgentView(t *testing.T) {
	reset(false)
	viper.Set("rsh-agent-view", "summary")

	body := map[string]any{
		"data": map[string]any{
			"project_name":       "Nubit",
			"tge_status":         "pre",
			"last_event_time":    "2026-06-01T00:00:00Z",
			"listing_exchanges":  []any{"Binance"},
			"large_raw_blob":     "dropped",
			"nested_unused_data": map[string]any{"foo": "bar"},
		},
	}

	transformed, err := transformResponseForCommand("market-tge", body)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"project_name":      "Nubit",
		"tge_status":        "pre",
		"last_event_time":   "2026-06-01T00:00:00Z",
		"listing_exchanges": []any{"Binance"},
	}, transformed)
}

func TestTransformResponseForCommandShapeSummary(t *testing.T) {
	reset(false)
	viper.Set("rsh-shape", true)

	body := map[string]any{
		"data": map[string]any{
			"contracts": map[string]any{"contracts": []any{}},
			"name":      "Bitcoin",
		},
		"meta": map[string]any{"request_id": "abc"},
	}

	transformed, err := transformResponseForCommand("project-detail", body)
	require.NoError(t, err)
	got := transformed.(map[string]any)

	assert.Equal(t, "project-detail", got["command"])
	assert.Equal(t, "object", got["body_type"])
	assert.Equal(t, []string{"data", "meta"}, got["top_keys"])
	assert.Equal(t, "object", got["data_type"])
	assert.Equal(t, []string{"contracts", "name"}, got["data_keys"])
	assert.Equal(t, []string{"contracts"}, got["suggested_views"])
}

func TestTransformResponseForCommandUnsupportedAgentViewReturnsError(t *testing.T) {
	reset(false)
	viper.Set("rsh-agent-view", "results")

	_, err := transformResponseForCommand("market-price", map[string]any{"data": []any{}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported --agent-view "results" for command "market-price"`)
}

func TestTransformResponseForCommandAgentViewPreservesErrorEnvelope(t *testing.T) {
	reset(false)
	viper.Set("rsh-agent-view", "results")

	body := map[string]any{
		"error": "rate limited",
		"code":  "rate_limit",
	}

	transformed, err := transformResponseForCommand("search-web", body)
	require.NoError(t, err)

	assert.Equal(t, body, transformed)
}
