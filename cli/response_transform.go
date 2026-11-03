package cli

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

var onchainTxHexQuantityFields = []string{
	"blockNumber",
	"chainId",
	"gas",
	"gasPrice",
	"maxFeePerGas",
	"maxPriorityFeePerGas",
	"nonce",
	"transactionIndex",
	"type",
	"value",
	"v",
	"yParity",
}

type agentViewFunc func(any) any

var agentViewRegistry = map[string]map[string]agentViewFunc{
	"market-tge": {
		"summary": marketTGESummaryView,
	},
	"project-detail": {
		"contracts": projectContractsView,
	},
	"search-web": {
		"results": searchWebResultsView,
	},
}

func transformResponseForCommand(command string, body any) (any, error) {
	if viper.GetBool("rsh-shape") {
		return responseShapeForCommand(command, body), nil
	}

	view := strings.TrimSpace(viper.GetString("rsh-agent-view"))
	if view != "" {
		views, ok := agentViewRegistry[command]
		if !ok {
			return nil, fmt.Errorf("unsupported --agent-view %q for command %q; supported views: %s", view, command, supportedAgentViews(command))
		}
		transform, ok := views[view]
		if !ok {
			return nil, fmt.Errorf("unsupported --agent-view %q for command %q; supported views: %s", view, command, supportedAgentViews(command))
		}
		if isErrorEnvelope(body) {
			return body, nil
		}
		return transform(body), nil
	}

	if command != "onchain-tx" {
		return body, nil
	}
	addOnchainTxDecimalFields(body)
	return body, nil
}

func addOnchainTxDecimalFields(body any) {
	switch v := body.(type) {
	case map[string]any:
		if data, ok := v["data"]; ok {
			addOnchainTxDecimalFields(data)
			return
		}
		addOnchainTxDecimalFieldsToRow(v)
	case []any:
		for _, item := range v {
			addOnchainTxDecimalFields(item)
		}
	}
}

func addOnchainTxDecimalFieldsToRow(tx map[string]any) {
	for _, field := range onchainTxHexQuantityFields {
		raw, ok := tx[field].(string)
		if !ok {
			continue
		}
		decimal, ok := hexQuantityToDecimalString(raw)
		if !ok {
			continue
		}

		decimalField := field + "Decimal"
		if _, exists := tx[decimalField]; !exists {
			tx[decimalField] = decimal
		}

		if field == "value" {
			if _, exists := tx["valueNativeDecimal"]; !exists {
				tx["valueNativeDecimal"] = weiDecimalToNativeDecimal(decimal)
			}
		}
	}
}

func hexQuantityToDecimalString(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if len(s) <= 2 || !strings.HasPrefix(strings.ToLower(s), "0x") {
		return "", false
	}

	n, ok := new(big.Int).SetString(s[2:], 16)
	if !ok {
		return "", false
	}
	return n.String(), true
}

func weiDecimalToNativeDecimal(decimalWei string) string {
	wei, ok := new(big.Int).SetString(decimalWei, 10)
	if !ok {
		return decimalWei
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	whole := new(big.Int)
	frac := new(big.Int)
	whole.QuoRem(wei, scale, frac)
	if frac.Sign() == 0 {
		return whole.String()
	}

	fracText := frac.String()
	if len(fracText) < 18 {
		fracText = strings.Repeat("0", 18-len(fracText)) + fracText
	}
	fracText = strings.TrimRight(fracText, "0")
	return whole.String() + "." + fracText
}

func searchWebResultsView(body any) any {
	results := firstListAtPaths(body,
		[]string{"data"},
		[]string{"data", "results"},
		[]string{"data", "items"},
		[]string{"results"},
		[]string{"items"},
	)
	if results == nil {
		return []any{}
	}

	out := make([]any, 0, len(results))
	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"title":       firstStringField(m, "title", "name"),
			"description": firstStringField(m, "description", "snippet", "summary", "content"),
			"url":         firstStringField(m, "url", "link", "href"),
		})
	}
	return out
}

func projectContractsView(body any) any {
	root := valueAtPath(body, []string{"data", "contracts"})
	if root == nil {
		root = valueAtPath(body, []string{"contracts"})
	}
	if root == nil {
		root = valueAtPath(body, []string{"data"})
	}

	rows := flattenContracts(root)
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, compactContract(row))
	}
	return out
}

func marketTGESummaryView(body any) any {
	data := valueAtPath(body, []string{"data"})
	if data == nil {
		data = body
	}

	switch v := data.(type) {
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, compactTGESummary(m))
			}
		}
		return out
	case map[string]any:
		return compactTGESummary(v)
	default:
		return data
	}
}

func responseShapeForCommand(command string, body any) any {
	data := valueAtPath(body, []string{"data"})
	return map[string]any{
		"command":         command,
		"body_type":       valueKind(body),
		"top_keys":        objectKeys(body),
		"data_type":       valueKind(data),
		"data_keys":       objectKeys(data),
		"sample":          sampleValue(data),
		"suggested_views": suggestedAgentViews(command),
	}
}

func supportedAgentViews(command string) string {
	views := suggestedAgentViews(command)
	if len(views) == 0 {
		return "none"
	}
	return strings.Join(views, ", ")
}

func suggestedAgentViews(command string) []string {
	viewsByName, ok := agentViewRegistry[command]
	if !ok {
		return []string{}
	}

	views := make([]string, 0, len(viewsByName))
	for name := range viewsByName {
		views = append(views, name)
	}
	sort.Strings(views)
	return views
}

func firstListAtPaths(body any, paths ...[]string) []any {
	for _, path := range paths {
		if list, ok := valueAtPath(body, path).([]any); ok {
			return list
		}
	}
	if list, ok := body.([]any); ok {
		return list
	}
	return nil
}

func valueAtPath(value any, path []string) any {
	current := value
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	return current
}

func firstStringField(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return nil
}

func flattenContracts(value any) []map[string]any {
	switch v := value.(type) {
	case []any:
		var rows []map[string]any
		for _, item := range v {
			rows = append(rows, flattenContracts(item)...)
		}
		return rows
	case map[string]any:
		if nested, ok := v["contracts"]; ok {
			return flattenContracts(nested)
		}
		if nested, ok := v["items"]; ok {
			return flattenContracts(nested)
		}
		if looksLikeContract(v) {
			return []map[string]any{v}
		}

		var rows []map[string]any
		keys := objectKeys(v)
		for _, key := range keys {
			rows = append(rows, flattenContracts(v[key])...)
		}
		return rows
	default:
		return nil
	}
}

func looksLikeContract(m map[string]any) bool {
	return firstStringField(m, "address", "contract_address", "contractAddress", "ca", "contract") != nil
}

func compactContract(m map[string]any) map[string]any {
	out := map[string]any{
		"chain":   firstStringField(m, "chain", "network", "blockchain", "chain_name", "chainName"),
		"address": firstStringField(m, "address", "contract_address", "contractAddress", "ca", "contract"),
		"symbol":  firstStringField(m, "symbol", "ticker"),
		"name":    firstStringField(m, "name", "token_name", "tokenName"),
	}
	if decimals, ok := m["decimals"]; ok {
		out["decimals"] = decimals
	}
	return out
}

func compactTGESummary(m map[string]any) map[string]any {
	keys := []string{
		"project", "project_name", "name", "symbol",
		"status", "tge_status", "stage",
		"date", "tge_date", "launch_date", "listing_date",
		"last_event_time", "next_event_time", "event_time",
		"exchanges", "listing_exchanges", "listings",
		"price", "price_usd", "token_price",
		"amount", "raise_amount", "valuation", "fdv",
		"public_sale", "unlock_percentage", "circulating_supply", "initial_circulating_supply",
	}

	out := make(map[string]any)
	for _, key := range keys {
		if value, ok := m[key]; ok {
			out[key] = value
		}
	}
	if len(out) > 0 {
		return out
	}

	for _, key := range objectKeys(m) {
		if nested, ok := m[key].(map[string]any); ok {
			for nestedKey, nestedValue := range compactTGESummary(nested) {
				out[nestedKey] = nestedValue
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	return sampleObject(m)
}

func isErrorEnvelope(value any) bool {
	m, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if _, ok := m["error"]; ok {
		return true
	}
	if _, ok := m["code"]; ok && m["data"] == nil {
		return true
	}
	return false
}

func valueKind(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, float32, int, int64, int32, uint, uint64, uint32:
		return "number"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func objectKeys(value any) []string {
	m, ok := value.(map[string]any)
	if !ok {
		return []string{}
	}

	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sampleValue(value any) any {
	switch v := value.(type) {
	case []any:
		limit := len(v)
		if limit > 2 {
			limit = 2
		}
		sample := make([]any, 0, limit)
		for i := 0; i < limit; i++ {
			sample = append(sample, sampleValue(v[i]))
		}
		return sample
	case map[string]any:
		return sampleObject(v)
	case string:
		runes := []rune(v)
		if len(runes) > 180 {
			return string(runes[:180]) + "..."
		}
		return v
	default:
		return v
	}
}

func sampleObject(m map[string]any) map[string]any {
	out := make(map[string]any)
	keys := objectKeys(m)
	if len(keys) > 8 {
		keys = keys[:8]
	}
	for _, key := range keys {
		out[key] = sampleValue(m[key])
	}
	return out
}
