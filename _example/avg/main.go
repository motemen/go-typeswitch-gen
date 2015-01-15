package main

import (
	"fmt"
)

func main() {
	var (
		ints   = []int{-1, 2, -3, 4, -5, 6}
		uints  = []uint{1, 2, 3, 4, 5, 6}
		floats = []float32{1.0, 1.1, 1.2, 1.3}
	)
	fmt.Printf("%#v avg: %v\n", ints, avg(ints))
	fmt.Printf("%#v avg: %v\n", uints, avg(uints))
	fmt.Printf("%#v avg: %v\n", floats, avg(floats))
}
