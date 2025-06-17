package main

//go:wasmimport conduit cube_request
func _cubeRequest() int32

//go:wasmimport conduit cube_response
func _cubeResponse(int32)

func main() {
	for {
		i := _cubeRequest()
		_cubeResponse(i * i * i)
	}
}
