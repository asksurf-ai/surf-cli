package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var contentTests = []struct {
	name   string
	types  []string
	ct     ContentType
	data   []byte
	pretty []byte
}{
	{"text", []string{"text/plain", "text/html"}, &Text{}, []byte("hello world"), nil},
	{"json", []string{"application/json", "foo+json"}, &JSON{}, []byte("{\"hello\":\"world\"}\n"), []byte("{\n  \"hello\": \"world\"\n}\n")},
	{"yaml", []string{"application/yaml", "foo+yaml"}, &YAML{}, []byte("hello: world\n"), nil},
	{"cbor", []string{"application/cbor", "foo+cbor"}, &CBOR{}, []byte("\xf6"), nil},
	{"msgpack", []string{"application/msgpack", "application/x-msgpack", "application/vnd.msgpack", "foo+msgpack"}, &MsgPack{}, []byte("\x81\xa5\x68\x65\x6c\x6c\x6f\xa5\x77\x6f\x72\x6c\x64"), nil},
	{"ion", []string{"application/ion", "foo+ion"}, &Ion{}, []byte("\xe0\x01\x00\xea\x0f"), []byte("null")},
}

// TestTableMarshal_DataEnvelopeAutoExtract verifies that Table.Marshal auto-extracts
// the "data" array from a Hermod-style { "data": [...], "meta": {...} } envelope.
func TestTableMarshal_DataEnvelopeAutoExtract(t *testing.T) {
	tbl := Table{}
	envelope := map[string]any{
		"data": []any{
			map[string]any{"symbol": "BTC", "price": 50000.0},
			map[string]any{"symbol": "ETH", "price": 3000.0},
		},
		"meta": map[string]any{"total": 2},
	}

	out, err := tbl.Marshal(envelope)
	require.NoError(t, err, "table should auto-extract data array from envelope")
	assert.Contains(t, string(out), "BTC")
	assert.Contains(t, string(out), "ETH")
}

// TestTableMarshal_PlainArray still works as before.
func TestTableMarshal_PlainArray(t *testing.T) {
	tbl := Table{}
	data := []any{
		map[string]any{"id": 1, "name": "alice"},
		map[string]any{"id": 2, "name": "bob"},
	}

	out, err := tbl.Marshal(data)
	require.NoError(t, err)
	assert.Contains(t, string(out), "alice")
	assert.Contains(t, string(out), "bob")
}

// TestTableMarshal_NonArrayNonEnvelopeErrors verifies non-array non-envelope input errors.
func TestTableMarshal_NonArrayNonEnvelopeErrors(t *testing.T) {
	tbl := Table{}
	_, err := tbl.Marshal(map[string]any{"foo": "bar"})
	assert.Error(t, err)
}

// TestTableMarshal_UnixTimestampFormatting verifies that fields ending in _time or _at
// with large numeric values are rendered as human-readable dates in table output.
func TestTableMarshal_UnixTimestampFormatting(t *testing.T) {
	tbl := Table{}
	data := []any{
		map[string]any{
			"block_time": float64(1740787200), // 2025-03-01 00:00:00 UTC
			"tx_hash":    "0xabc",
		},
	}

	out, err := tbl.Marshal(data)
	require.NoError(t, err)
	s := string(out)
	// Should contain a human-readable date, not raw number
	assert.True(t, strings.Contains(s, "2025"), "expected human-readable year in output, got: %s", s)
	assert.False(t, strings.Contains(s, "1.74"), "should not contain scientific notation")
}

func TestContentTypes(parent *testing.T) {
	for _, tt := range contentTests {
		parent.Run(tt.name, func(t *testing.T) {
			for _, typ := range tt.types {
				assert.True(t, tt.ct.Detect(typ))
			}

			assert.False(t, tt.ct.Detect("bad-content-type"))

			var data any
			err := tt.ct.Unmarshal(tt.data, &data)
			assert.NoError(t, err)

			b, err := tt.ct.Marshal(data)
			assert.NoError(t, err)

			if tt.pretty != nil {
				if p, ok := tt.ct.(PrettyMarshaller); ok {
					b, err := p.MarshalPretty(data)
					assert.NoError(t, err)
					assert.Equal(t, tt.pretty, b)
				} else {
					t.Fatal("not a pretty marshaller")
				}
			}

			assert.Equal(t, tt.data, b)
		})
	}
}
