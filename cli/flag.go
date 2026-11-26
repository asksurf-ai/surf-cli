package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// AddGlobalFlag will make a new global flag on the root command.
func AddGlobalFlag(name, short, description string, defaultValue any, multi bool) {
	viper.SetDefault(name, defaultValue)

	flags := Root.PersistentFlags()
	switch defaultValue.(type) {
	case bool:
		if multi {
			flags.BoolSliceP(name, short, viper.Get(name).([]bool), description)
			GlobalFlags.BoolSliceP(name, short, viper.Get(name).([]bool), description)
		} else {
			flags.BoolP(name, short, viper.GetBool(name), description)
			GlobalFlags.BoolP(name, short, viper.GetBool(name), description)
		}
	case time.Duration:
		if multi {
			panic(fmt.Errorf("unsupported float slice param"))
		} else {
			flags.DurationP(name, short, viper.GetDuration(name), description)
			GlobalFlags.DurationP(name, short, viper.GetDuration(name), description)
		}
	case int, int16, int32, int64, uint16, uint32, uint64:
		if multi {
			flags.IntSliceP(name, short, viper.Get(name).([]int), description)
			GlobalFlags.IntSliceP(name, short, viper.Get(name).([]int), description)
		} else {
			flags.IntP(name, short, viper.GetInt(name), description)
			GlobalFlags.IntP(name, short, viper.GetInt(name), description)
		}
	case float32, float64:
		if multi {
			panic(fmt.Errorf("unsupported float slice param"))
		} else {
			flags.Float64P(name, short, viper.GetFloat64(name), description)
			GlobalFlags.Float64P(name, short, viper.GetFloat64(name), description)
		}
	default:
		if multi {
			v := viper.Get(name)
			if s, ok := v.(string); ok {
				// Probably loaded from the environment.
				v = strings.Split(s, ",")
				viper.Set(name, v)
			}
			flags.StringArrayP(name, short, v.([]string), description)
			GlobalFlags.StringArrayP(name, short, v.([]string), description)
		} else {
			flags.StringP(name, short, fmt.Sprintf("%v", viper.Get(name)), description)
			GlobalFlags.StringP(name, short, fmt.Sprintf("%v", viper.Get(name)), description)
		}
	}

	viper.BindPFlag(name, flags.Lookup(name))
}

// EagerArgs returns the args that can hold global flags, for code that has to
// look at global flags before the API operations and their flags are
// registered (the early GlobalFlags parse in Run, the base URL override in
// main). Without operation flags pflag can't know that e.g. `--q` takes a
// value and would read `--q "-twitter bitcoin"` as `-t "witter bitcoin"`.
// Long flags that aren't global are dropped here together with their value.
func EagerArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		name, ok := strings.CutPrefix(arg, "--")
		if !ok {
			out = append(out, arg)
			continue
		}
		name, _, hasValue := strings.Cut(name, "=")
		if GlobalFlags.Lookup(name) != nil {
			out = append(out, arg)
			continue
		}
		if hasValue || i+1 >= len(args) {
			continue
		}
		if f := Root.PersistentFlags().Lookup(name); f != nil {
			// Root flags like --json are known up front, so we know whether
			// the next arg is their value.
			if f.NoOptDefVal == "" {
				i++
			}
			continue
		}
		// Most likely an operation flag. Treat the next arg as its value
		// unless it is clearly a flag of its own, which is what a bool
		// operation flag followed by `--rsh-verbose` or `-p prod` looks like.
		if !looksLikeGlobalFlag(args[i+1]) {
			i++
		}
	}
	return out
}

func looksLikeGlobalFlag(arg string) bool {
	if strings.HasPrefix(arg, "--") {
		return true
	}
	return len(arg) == 2 && arg[0] == '-' && GlobalFlags.ShorthandLookup(arg[1:]) != nil
}
