package e

import (
	"io"
)

type T interface{}
type S interface{}

type foo struct{}

type in1 map[string][]io.Reader
type in2 map[int]bool
type in3 []chan<- *foo
type in4 []struct{}
type in5 *foo
type in6 func(int)
type in7 func(bool) (io.Reader, error)
type in8 struct{ foo []byte }

func main() {
	Foo(map[string][]io.Reader{})
	Foo(map[int]bool{})
	Foo(make([]chan<- *foo, 0))
	Foo([]struct{}{})
	Foo(func(int) (bool, error) { return true, nil })
}

func Foo(x interface{}) {
	switch x := x.(type) {
	// in1
	case map[string]T:
		var r T // <-- T here
		for _, v := range x {
			r = v
		}
		_ = r

	// in2
	case map[T]bool:
		var keys []T = make([]T, 0)
		for k := range x {
			keys = append(keys, k)
		}
		_ = keys

	// in3
	case []chan<- T:
		var t1, t2 T
		for _, c := range x {
			c <- t1
			c <- t2
		}

	// in4
	case []T:
		var t T = x[0]
		_ = t

	// in5
	case *T:
		var t T = *x
		_ = t

	// in6
	case func(T):
		var t *T
		x(*t)

	// in7
	case func(T) (S, error):
		var t T
		var s S
		s, _ = x(t)
		_ = s

	// in8
	case struct{ foo T }:
		var t T = x.foo
		_ = t
	}
}
