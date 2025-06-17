package main

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var res int32

func BenchmarkCube(b *testing.B) {
	ctx := context.Background()

	// Create a Wasm runtime, set up WASI.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	m, err := NewCubeModule(ctx, r, path)
	if err != nil {
		b.Fatalf("failed to create module: %v", err)
	}
	defer m.Close(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res = m.Cube(2)
	}
}
