package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var path = "module/module.wasm" // Path to the WASM file to be executed.

type MyModule struct {
	m   api.Module
	add api.Function
}

func (my *MyModule) Add(i, j int32) int32 {
	out, err := my.add.Call(context.Background(), api.EncodeI32(i), api.EncodeI32(j))
	if err != nil {
		panic(fmt.Sprintf("failed to call add: %v", err))
	}
	return api.DecodeI32(out[0])
}

func (my *MyModule) Close(ctx context.Context) error {
	return my.m.Close(ctx)
}

func main() {
	ctx := context.Background()

	// Create a Wasm runtime, set up WASI.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	m, err := NewModule(ctx, r, path)
	if err != nil {
		panic(err)
	}

	var a, b int32 = 1, 2
	c := m.Add(a, b)

	fmt.Printf("add(%d, %d) = %d\n", a, b, c)

	d := m.Add(b, c)
	fmt.Printf("add(%d, %d) = %d\n", b, c, d)

	err = m.Close(ctx)
	if err != nil {
		panic(err)
	}
}

func NewModule(ctx context.Context, r wazero.Runtime, path string) (*MyModule, error) {

	// Configure the module to initialize the reactor.
	config := wazero.NewModuleConfig().WithStartFunctions()

	// Instantiate the module.
	wasmFile, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read WASM file: %w", err)
	}

	wasmModule, err := r.InstantiateWithConfig(ctx, wasmFile, config)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate wasm module: %w", err)
	}

	fn1 := wasmModule.ExportedFunction("_initialize")
	_, err = fn1.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call _initialize: %w", err)
	}

	return &MyModule{
		m:   wasmModule,
		add: wasmModule.ExportedFunction("add"),
	}, nil
}
