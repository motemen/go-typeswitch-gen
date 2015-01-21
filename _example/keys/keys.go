//go:generate tsgen -w expand $GOFILE
//go:generate goimports -w $GOFILE

package main

import "fmt"

type T interface{}

func keys(m interface{}) []string {
	switch m := m.(type) {
	case map[string]T:
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		return keys
	default:
		panic(fmt.Sprintf("unexpected type: %T", m))
	}
}
