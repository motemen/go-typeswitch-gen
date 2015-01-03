package E

import "testing"

func TestForeach(t *testing.T) {
	foreach([]string{"a", "bb"}, func(i int, s string) {
		t.Log(i, s)
	})

	foreach([]bool{true, false}, func(i int, b bool) {
		t.Log(i, b)
	})
}
