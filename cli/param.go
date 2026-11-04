package cli

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/spf13/pflag"
)

// Style is an encoding style for parameters.
type Style int

const (
	// StyleSimple corresponds to OpenAPI 3 simple parameters
	StyleSimple Style = iota

	// StyleForm corresponds to OpenAPI 3 form parameters
	StyleForm
)

func typeConvert(from, to any) any {
	return reflect.ValueOf(from).Convert(reflect.TypeOf(to)).Interface()
}

// Param represents an API operation input parameter.
type Param struct {
	Type        string   `json:"type" yaml:"type"`
	Name        string   `json:"name" yaml:"name"`
	DisplayName string   `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Style       Style    `json:"style,omitempty" yaml:"style,omitempty"`
	Explode     bool     `json:"explode,omitempty" yaml:"explide,omitempty"`
	Required    bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any      `json:"default,omitempty" yaml:"default,omitempty"`
	Example     any      `json:"example,omitempty" yaml:"example,omitempty"`
	Enum        []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty" yaml:"maximum,omitempty"`
}

// enumStringValue is a pflag.Value whose Type() reports a custom placeholder
// like "{a|b|c}". pflag's UnquoteUsage falls back to Value.Type() when the
// usage string has no back-quoted word, so this lets us render enum choices
// directly in the flag's type column.
type enumStringValue struct {
	val     *string
	typeStr string
}

func (e *enumStringValue) String() string     { return *e.val }
func (e *enumStringValue) Set(s string) error { *e.val = s; return nil }
func (e *enumStringValue) Type() string       { return e.typeStr }

// Parse the parameter from a string input (e.g. command line argument)
func (p Param) Parse(value string) (any, error) {
	// TODO: parse based on the type, used mostly for path parameter parsing
	// which is almost always a string anyway.
	return value, nil
}

// Serialize the parameter based on the type/style/explode configuration.
func (p Param) Serialize(value any) []string {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		value = v.Interface()
	}

	switch p.Type {
	case "boolean", "integer", "number", "string":
		switch p.Style {
		case StyleForm:
			return []string{fmt.Sprintf("%s=%v", p.Name, value)}
		case StyleSimple:
			return []string{fmt.Sprintf("%v", value)}
		}

	case "array[boolean]", "array[integer]", "array[number]", "array[string]":
		tmp := []string{}
		switch p.Style {
		case StyleForm:
			for i := 0; i < v.Len(); i++ {
				item := v.Index(i)
				if p.Explode {
					tmp = append(tmp, fmt.Sprintf("%v", item.Interface()))
				} else {
					if len(tmp) == 0 {
						tmp = append(tmp, "")
					}

					tmp[0] += fmt.Sprintf("%v", item.Interface())
					if i < v.Len()-1 {
						tmp[0] += ","
					}
				}
			}
		case StyleSimple:
			tmp = append(tmp, "")
			for i := 0; i < v.Len(); i++ {
				item := v.Index(i)
				tmp[0] += fmt.Sprintf("%v", item.Interface())
				if i < v.Len()-1 {
					tmp[0] += ","
				}
			}
		}
		return tmp
	}

	return nil
}

// OptionName returns the commandline option name for this parameter.
func (p Param) OptionName() string {
	name := p.Name
	if p.DisplayName != "" {
		name = p.DisplayName
	}
	return strcase.ToDelimited(name, '-')
}

// HelpfulExampleFor judges redundancy against the description that will
// actually be shown. Compact output drops descriptions, so an example that was
// "already in the description" must reappear there.
func (p Param) HelpfulExampleFor(desc string) any {
	if p.Example == nil {
		return nil
	}
	ex := fmt.Sprintf("%v", p.Example)
	if ex == "" || p.exampleRedundant(desc, ex) {
		return nil
	}
	return p.Example
}

// exampleRedundant reports whether showing the example would repeat something
// the reader already has: the same value as the default, or text that already
// appears in the description. Most Surf parameters carry both an enum and an
// example, so without this check the hint adds tokens instead of information.
func (p Param) exampleRedundant(desc, example string) bool {
	if strings.Contains(desc, "example:") || strings.Contains(desc, "Example:") {
		return true
	}
	if strings.Contains(desc, example) {
		return true
	}
	for _, v := range p.Enum {
		if v == example {
			return true
		}
	}
	return p.Default != nil && fmt.Sprintf("%v", p.Default) == example
}

// SupportsFlag reports whether AddFlag can register this parameter type. The
// catalog uses it so it never advertises a flag the command does not have.
func (p Param) SupportsFlag() bool {
	switch p.Type {
	case "boolean", "integer", "number", "string",
		"array[boolean]", "array[integer]", "array[string]":
		return true
	}
	return false
}

// AddFlag adds a new option flag to a command's flag set for this parameter.
func (p Param) AddFlag(flags *pflag.FlagSet) any {
	name := p.OptionName()
	if !p.SupportsFlag() {
		// The catalog uses SupportsFlag too, so the flag is simply absent
		// everywhere instead of logging on every command startup.
		return nil
	}
	def := p.Default
	desc := p.Description
	if p.Required {
		if desc != "" {
			desc += " (required)"
		} else {
			desc = "(required)"
		}
	}

	switch p.Type {
	case "boolean":
		if def == nil {
			def = false
		}
		return flags.Bool(name, def.(bool), desc)
	case "integer":
		if def == nil {
			def = 0
		}
		return flags.Int(name, typeConvert(def, 0).(int), desc)
	case "number":
		if def == nil {
			def = 0.0
		}
		return flags.Float64(name, typeConvert(def, float64(0.0)).(float64), desc)
	case "string":
		if def == nil {
			def = ""
		}
		if len(p.Enum) > 0 {
			placeholder := "{" + strings.Join(p.Enum, "|") + "}"
			// Skip the custom placeholder if it would be too wide: pflag pads
			// every flag line to the widest type column, so one long enum
			// would push every description onto its own line. Wide enums are
			// still listed in the description by appendSchemaHints.
			if len(placeholder) <= 40 {
				v := def.(string)
				flags.Var(&enumStringValue{val: &v, typeStr: placeholder}, name, desc)
				return &v
			}
		}
		return flags.String(name, def.(string), desc)
	case "array[boolean]":
		if def == nil {
			def = []bool{}
		} else if anyList, ok := def.([]any); ok {
			typed := make([]bool, len(anyList))
			for i, item := range anyList {
				typed[i] = item.(bool)
			}
			def = typed
		}
		return flags.BoolSlice(name, def.([]bool), desc)
	case "array[integer]":
		if def == nil {
			def = []int{}
		} else if anyList, ok := def.([]any); ok {
			typed := make([]int, len(anyList))
			for i, item := range anyList {
				typed[i] = typeConvert(item, 0).(int)
			}
			def = typed
		}
		return flags.IntSlice(name, def.([]int), desc)
	// Float slices aren't implemented in the pflag package...
	// if def == nil {
	// 	def = []float64{}
	// }
	// return flags.Float64Slice(p.Name, def.([]float64), p.Description)
	case "array[string]":
		if def == nil {
			def = []string{}
		} else {
			tmp := []string{}
			for _, item := range def.([]any) {
				tmp = append(tmp, item.(string))
			}
			def = tmp
		}
		return flags.StringSlice(name, def.([]string), desc)
	}

	return nil
}
