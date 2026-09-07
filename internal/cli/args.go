// Package cli implements the mdq subcommands.
package cli

import (
	"fmt"
	"strconv"
	"strings"
)

type argSet struct {
	pos  []string
	flag map[string]string
}

// parseArgs splits argv into positional arguments and flags. Flags may appear
// anywhere. valued lists the flags that take a value, either as "--flag value"
// or "--flag=value". Every other known flag is a boolean and records "true".
func parseArgs(argv []string, known, valued map[string]bool) (argSet, error) {
	a := argSet{flag: map[string]string{}}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			a.pos = append(a.pos, argv[i+1:]...)
			break
		}
		if len(arg) < 2 || !strings.HasPrefix(arg, "-") {
			a.pos = append(a.pos, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		val := ""
		hasVal := false
		if k := strings.Index(name, "="); k >= 0 {
			name, val, hasVal = name[:k], name[k+1:], true
		}
		if !known[name] {
			return a, fmt.Errorf("unknown flag %q", arg)
		}
		if valued[name] {
			if !hasVal {
				if i+1 >= len(argv) {
					return a, fmt.Errorf("flag %q needs a value", arg)
				}
				i++
				val = argv[i]
			}
			a.flag[name] = val
			continue
		}
		if !hasVal {
			val = "true"
		}
		a.flag[name] = val
	}
	return a, nil
}

func (a argSet) has(name string) bool {
	v, ok := a.flag[name]
	return ok && v != "false"
}

func (a argSet) str(name, def string) string {
	if v, ok := a.flag[name]; ok {
		return v
	}
	return def
}

func (a argSet) num(name string, def int) (int, error) {
	v, ok := a.flag[name]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("flag --%s needs a number, got %q", name, v)
	}
	return n, nil
}

func flagSet(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}
