wasm:
	cd exported/module; GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o module.wasm
	cd imported/add/module; GOOS=wasip1 GOARCH=wasm go build -o module.wasm
	cd imported/cube/module; GOOS=wasip1 GOARCH=wasm go build -o module.wasm

bench: wasm
	go test -bench=. ./exported ./imported/add ./imported/cube
