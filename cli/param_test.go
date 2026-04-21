package cli

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

var paramInputs = []struct {
	Name     string
	Type     string
	Style    Style
	Explode  bool
	Value    any
	Expected []string
}{
	{"bool-simple", "boolean", StyleSimple, false, true, []string{"true"}},
	{"bool-form", "boolean", StyleForm, false, true, []string{"test=true"}},
	{"int-simple", "integer", StyleSimple, false, 123, []string{"123"}},
	{"int-form", "integer", StyleForm, false, 123, []string{"test=123"}},
	{"num-simple", "number", StyleSimple, false, 123.4, []string{"123.4"}},
	{"num-form", "number", StyleForm, false, 123.4, []string{"test=123.4"}},
	{"str-simple", "string", StyleSimple, false, "hello", []string{"hello"}},
	{"str-form", "string", StyleForm, false, "hello", []string{"test=hello"}},
	{"arr-bool-simple", "array[boolean]", StyleSimple, false, []bool{true, false}, []string{"true,false"}},
	{"arr-bool-form", "array[boolean]", StyleForm, false, []bool{true, false}, []string{"true,false"}},
	{"arr-bool-form-explode", "array[boolean]", StyleForm, true, []bool{true, false}, []string{"true", "false"}},
	{"arr-int-simple", "array[integer]", StyleSimple, false, []int{123, 456}, []string{"123,456"}},
	{"arr-int-form", "array[integer]", StyleForm, false, []int{123, 456}, []string{"123,456"}},
	{"arr-int-form-explode", "array[integer]", StyleForm, true, []int{123, 456}, []string{"123", "456"}},
	{"arr-str-simple", "array[string]", StyleSimple, false, []string{"one", "two"}, []string{"one,two"}},
	{"arr-str-form", "array[string]", StyleForm, false, []string{"one", "two"}, []string{"one,two"}},
	{"arr-str-form-explode", "array[string]", StyleForm, true, []string{"one", "two"}, []string{"one", "two"}},
}

func TestParamSerialize(t *testing.T) {
	for _, input := range paramInputs {
		t.Run(input.Name, func(t *testing.T) {
			p := Param{
				Name:    "test",
				Type:    input.Type,
				Style:   input.Style,
				Explode: input.Explode,
			}

			serialized := p.Serialize(input.Value)
			assert.Equal(t, input.Expected, serialized)
		})
	}
}

func TestParamFlag(t *testing.T) {
	for _, input := range paramInputs {
		t.Run(input.Name, func(t *testing.T) {
			p := Param{
				Name:    "test",
				Type:    input.Type,
				Style:   input.Style,
				Explode: input.Explode,
			}

			flags := pflag.NewFlagSet("", pflag.PanicOnError)
			p.AddFlag(flags)

			assert.NotNil(t, flags.Lookup("test"))
		})
	}
}

func TestParamFlagEnumPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		enum     []string
		wantType string
	}{
		{"no enum", nil, "string"},
		{"two values", []string{"tier", "portfolio_count"}, "{tier|portfolio_count}"},
		{"three values", []string{"asc", "desc", "none"}, "{asc|desc|none}"},
		{"too wide falls back to string", []string{
			"one_really_long_value", "another_really_long_value",
		}, "string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Param{
				Name: "test",
				Type: "string",
				Enum: tt.enum,
			}
			flags := pflag.NewFlagSet("", pflag.PanicOnError)
			p.AddFlag(flags)

			flag := flags.Lookup("test")
			assert.NotNil(t, flag)
			assert.Equal(t, tt.wantType, flag.Value.Type())
		})
	}
}

func TestParamFlagRequiredSuffix(t *testing.T) {
	tests := []struct {
		name        string
		required    bool
		description string
		wantUsage   string
	}{
		{"optional with description", false, "Some description", "Some description"},
		{"optional without description", false, "", ""},
		{"required with description", true, "Some description", "Some description (required)"},
		{"required without description", true, "", "(required)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Param{
				Name:        "test",
				Type:        "string",
				Description: tt.description,
				Required:    tt.required,
			}
			flags := pflag.NewFlagSet("", pflag.PanicOnError)
			p.AddFlag(flags)

			flag := flags.Lookup("test")
			assert.NotNil(t, flag)
			assert.Equal(t, tt.wantUsage, flag.Usage)
		})
	}
}
