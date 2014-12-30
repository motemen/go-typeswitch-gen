package main

import (
	"fmt"
)

func main() {
	intMap := map[string]int{
		"foo": 1,
		"bar": 2,
	}
	boolMap := map[string]bool{
		"a": true,
		"b": false,
	}

	fmt.Println(mapKeys(intMap))
	fmt.Println(mapKeys(boolMap))
}
