//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jclark/gps2phc/internal/ubx/cfg/schema"
)

func main() {
	bytes, err := os.ReadFile("schema/cfg.yaml")
	if err == nil {
		schema, err := schema.UnmarshalYAML(bytes)
		if err == nil {
			err = os.WriteFile("schema.go", []byte(fmtSchema(schema)), 0644)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
		os.Exit(1)
	}
}

func fmtSchema(schema map[string]map[string]schema.Item) string {
	groups := make([]string, 0, len(schema))
	for name, m := range schema {
		groups = append(groups, fmtGroup(name, m))
	}
	sort.Strings(groups)
	return fmt.Sprintf("package cfg\n\nvar schema = MustNewSchema(map[string]map[string]Desc{\n%s})\n", strings.Join(groups, ""))
}

func fmtGroup(name string, m map[string]schema.Item) string {
	lines := make([]string, 0, len(m))
	for name, item := range m {
		lines = append(lines, fmtItem(name, item))
	}
	sort.Strings(lines)
	return fmt.Sprintf("\t%q:{\n%s\t},\n", name, strings.Join(lines, ""))
}

func fmtItem(name string, item schema.Item) string {
	return fmt.Sprintf("\t\t%q: %s(0x%x%s),\n", name, item.Type[:1], item.Key, fmtConsts(item.Constants))
}

func fmtConsts(cMap map[string]schema.Constant) string {
	if cMap == nil {
		return ""
	}
	n := uint64(0)
	for _, c := range cMap {
		i := c.Value
		if i >= n {
			n = i + 1
		}
	}
	consts := make([]string, n)
	for s, c := range cMap {
		consts[c.Value] = s
	}
	args := ""
	for _, s := range consts {
		args += fmt.Sprintf(", %q", s)
	}
	return args
}
