//go:generate tsgen -w expand $GOFILE

package main

import (
	"fmt"
)

// +tsgen typevar
type NumT float64

func avg(a interface{}) float64 {
	switch a := a.(type) {
	case []NumT:
		var sum NumT
		for _, x := range a {
			sum = sum + x
		}
		return float64(sum / NumT(len(a)))

	default:
		panic(fmt.Sprintf("unpexpected type: %T", a))
	}
}
