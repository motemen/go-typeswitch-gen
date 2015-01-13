package main

import (
	"fmt"
)

func main() {
	fmt.Println(avg([]int{-1, 2, -3, 4, -5, 6}))
	fmt.Println(avg([]uint{1, 2, 3, 4, 5, 6}))
	fmt.Println(avg([]float32{1.0, 1.1, 1.2, 1.3}))
}
