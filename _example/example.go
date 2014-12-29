package main

import (
	"fmt"
)

type T interface{}
type S T

var mapKeys1 = func(m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func mapKeys2(m interface{}) []string {
	switch m := m.(type) {
	default:
		panic(fmt.Sprintf("unexpected value of type %T", m))

	case map[string]T:
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		return keys
	}
}

func main() {
	intMap := map[string]int{
		"foo": 1,
		"bar": 2,
	}
	boolMap := map[string]bool{
		"a": true,
		"b": false,
	}

	fmt.Println(mapKeys2(intMap))
	fmt.Println(mapKeys2(boolMap))
}
