package main

//go:wasmimport conduit add_request
func _addRequest(*int32, *int32)

//go:wasmimport conduit add_response
func _addResponse(int32)

func main() {
	for {
		var i, j int32
		_addRequest(&i, &j)
		_addResponse(i + j)
	}
}
