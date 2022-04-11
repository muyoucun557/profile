package main

import (
	"fmt"
	"unsafe"
)

type A struct {
	a uint32
	b uint32
}

func main() {
	fmt.Println(unsafe.Offsetof(A{}.a)) // 输出0
	fmt.Println(unsafe.Offsetof(A{}.b)) // 输出4，表示a占用了4个字节
}
