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

type Module struct {
	m    api.Module
	add  api.Function
	cube api.Function
}

func (my *Module) Add(i, j int32) int32 {
	out, err := my.add.Call(context.Background(), api.EncodeI32(i), api.EncodeI32(j))
	if err != nil {
		panic(fmt.Sprintf("failed to call add: %v", err))
	}
	return api.DecodeI32(out[0])
}

func (my *Module) Cube(i int32) int32 {
	out, err := my.cube.Call(context.Background(), api.EncodeI32(i))
	if err != nil {
		panic(fmt.Sprintf("failed to call cube: %v", err))
	}
	return api.DecodeI32(out[0])
}

func (my *Module) Close(ctx context.Context) error {
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

	e := m.Cube(d)
	fmt.Printf("cube(%d) = %d\n", d, e)

	f := m.Cube(e)
	fmt.Printf("cube(%d) = %d\n", e, f)

	err = m.Close(ctx)
	if err != nil {
		panic(err)
	}
}

func NewModule(ctx context.Context, r wazero.Runtime, path string) (*Module, error) {
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

	return &Module{
		m:    wasmModule,
		add:  wasmModule.ExportedFunction("add"),
		cube: wasmModule.ExportedFunction("cube"),
	}, nil
}
