package main

import (
	"context"
	"fmt"
	"os"

	"github.com/conduitio/conduit/pkg/foundation/cerrors"
	"github.com/stealthrocket/wazergo"
	"github.com/stealthrocket/wazergo/types"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

var path = "module/module.wasm" // Path to the WASM file to be executed.

type MyModule struct {
	req chan tuple[int32, int32]
	res chan int32

	m api.Module
}

func (my *MyModule) Add(i, j int32) int32 {
	my.req <- tuple[int32, int32]{i, j}
	return <-my.res
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
	compiledHostModule, err := wazergo.Compile(ctx, r, hostModule)
	if err != nil {
		return nil, fmt.Errorf("failed to compile host module: %w", err)
	}

	// Configure the module to initialize the reactor.
	config := wazero.NewModuleConfig().WithStartFunctions()

	req := make(chan tuple[int32, int32], 1)
	res := make(chan int32, 1)

	ins, err := compiledHostModule.Instantiate(ctx, hostModuleOptions(req, res))
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate host module: %w", err)
	}
	ctx = wazergo.WithModuleInstance(ctx, ins)

	// Instantiate the module.
	wasmFile, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read WASM file: %w", err)
	}

	wasmModule, err := r.InstantiateWithConfig(ctx, wasmFile, config)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate wasm module: %w", err)
	}

	go runModule(ctx, wasmModule)
	return &MyModule{
		req: req,
		res: res,
		m:   wasmModule,
	}, nil
}

// run is the main loop of the WASM module. It runs in a goroutine and blocks
// until the module is closed.
func runModule(
	ctx context.Context,
	module api.Module,
) {
	_, err := module.ExportedFunction("_start").Call(ctx)

	// main function returned, close the module right away
	_ = module.Close(ctx)

	if err != nil {
		var exitErr *sys.ExitError
		if cerrors.As(err, &exitErr) {
			if exitErr.ExitCode() == 0 { // All good
				err = nil
			}
		}
	}

	if err != nil {
		fmt.Println("ERR: Wasm module stopped with error:", err)
	} else {
		fmt.Println("Wasm module stopped successfully")
	}
}

// hostModule declares the host module that is exported to the WASM module. The
// host module is used to communicate between the WASM module (processor) and Conduit.
var hostModule wazergo.HostModule[*hostModuleInstance] = hostModuleFunctions{
	"add_request":  F2((*hostModuleInstance).addRequest),
	"add_response": F1((*hostModuleInstance).addResponse),
}

// hostModuleFunctions type implements HostModule, providing the module name,
// map of exported functions, and the ability to create instances of the module
// type.
type hostModuleFunctions wazergo.Functions[*hostModuleInstance]

// Name returns the name of the module.
func (f hostModuleFunctions) Name() string {
	return "conduit"
}

// Functions is a helper that returns the exported functions of the module.
func (f hostModuleFunctions) Functions() wazergo.Functions[*hostModuleInstance] {
	return (wazergo.Functions[*hostModuleInstance])(f)
}

// Instantiate creates a new instance of the module. This is called by the
// runtime when a new instance of the module is created.
func (f hostModuleFunctions) Instantiate(_ context.Context, opts ...hostModuleOption) (*hostModuleInstance, error) {
	mod := &hostModuleInstance{}
	wazergo.Configure(mod, opts...)
	return mod, nil
}

type hostModuleOption = wazergo.Option[*hostModuleInstance]

func hostModuleOptions(
	chanRequests chan tuple[int32, int32],
	chanResponses chan int32,
) hostModuleOption {
	return wazergo.OptionFunc(func(m *hostModuleInstance) {
		m.addRequests = chanRequests
		m.addResponses = chanResponses
	})
}

// hostModuleInstance is used to maintain the state of our module instance.
type hostModuleInstance struct {
	addRequests  chan tuple[int32, int32]
	addResponses chan int32
}

func (*hostModuleInstance) Close(context.Context) error { return nil }

func (m *hostModuleInstance) addRequest(ctx context.Context, i, j types.Pointer[types.Int32]) {
	req := <-m.addRequests
	i.Store(types.Int32(req.V1))
	j.Store(types.Int32(req.V2))
}

func (m *hostModuleInstance) addResponse(ctx context.Context, out types.Int32) {
	m.addResponses <- int32(out)
}

// F2R is the Function constructor for functions accepting no parameters and returning 2.
func F2R[T any, R1 types.Result, R2 types.Result](fn func(T, context.Context) (R1, R2)) wazergo.Function[T] {
	var ret1 R1
	var ret2 R2
	return wazergo.Function[T]{
		Results: []types.Value{ret1, ret2},
		Func: func(this T, ctx context.Context, module api.Module, stack []uint64) {
			r1, r2 := fn(this, ctx)
			r1.StoreValue(module.Memory(), stack)
			r2.StoreValue(module.Memory(), stack[len(r1.ValueTypes()):])
		},
	}
}

// F1 is the Function constructor for functions accepting one parameter.
func F1[T any, P types.Param[P]](fn func(T, context.Context, P)) wazergo.Function[T] {
	var arg P
	return wazergo.Function[T]{
		Params:  []types.Value{arg},
		Results: []types.Value{},
		Func: func(this T, ctx context.Context, module api.Module, stack []uint64) {
			var arg P
			var memory = module.Memory()
			fn(this, ctx, arg.LoadValue(memory, stack))
		},
	}
}

// F1 is the Function constructor for functions accepting two parameters.
func F2[
	T any,
	P1 types.Param[P1],
	P2 types.Param[P2],
](fn func(T, context.Context, P1, P2)) wazergo.Function[T] {
	var arg1 P1
	var arg2 P2
	params1 := arg1.ValueTypes()
	params2 := arg2.ValueTypes()
	a := len(params1)
	b := len(params2) + a
	return wazergo.Function[T]{
		Params:  []types.Value{arg1, arg2},
		Results: []types.Value{},
		Func: func(this T, ctx context.Context, module api.Module, stack []uint64) {
			var arg1 P1
			var arg2 P2
			var memory = module.Memory()
			fn(this, ctx,
				arg1.LoadValue(memory, stack[0:a:a]),
				arg2.LoadValue(memory, stack[a:b:b]),
			)
		},
	}
}

type tuple[T1, T2 any] struct {
	V1 T1
	V2 T2
}
