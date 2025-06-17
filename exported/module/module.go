package main

//go:wasmexport add
func add(a, b int32) int32 {
	return a + b
}

//go:wasmexport cube
func cube(a int32) int32 {
	return a * a * a
}

func main() {}
