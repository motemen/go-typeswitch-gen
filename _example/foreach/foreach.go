//go:generate tsgen -w $GOFILE
package E

import "fmt"

type T interface{}

func foreach(a interface{}, cb interface{}) {
	switch a := a.(type) {
	case []T:
		switch cb := cb.(type) {
		case func(int, T):
			for i, e := range a {
				cb(i, e)
			}
		}

	default:
		panic(fmt.Sprintf("unpexpected type of %T and %T", a, cb))
	}
}
