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

type CubeModule struct {
	req chan<- int32
	res <-chan int32

	m api.Module
}

func (my *CubeModule) Cube(i int32) int32 {
	my.req <- i
	return <-my.res
}

func (my *CubeModule) Close(ctx context.Context) error {
	return my.m.Close(ctx)
}

func main() {
	ctx := context.Background()

	// Create a Wasm runtime, set up WASI.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	m, err := NewCubeModule(ctx, r, path)
	if err != nil {
		panic(err)
	}

	var a int32 = 2
	b := m.Cube(a)

	fmt.Printf("cube(%d) = %d\n", a, b)

	c := m.Cube(b)
	fmt.Printf("cube(%d) = %d\n", b, c)

	err = m.Close(ctx)
	if err != nil {
		panic(err)
	}
}

func NewCubeModule(ctx context.Context, r wazero.Runtime, path string) (*CubeModule, error) {
	compiledHostModule, err := wazergo.Compile(ctx, r, hostModule)
	if err != nil {
		return nil, fmt.Errorf("failed to compile host module: %w", err)
	}

	// Configure the module to initialize the reactor.
	config := wazero.NewModuleConfig().WithStartFunctions()

	req := make(chan int32, 1)
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
	return &CubeModule{
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
	"cube_request":  wazergo.F0((*hostModuleInstance).cubeRequest),
	"cube_response": F1((*hostModuleInstance).cubeResponse),
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
	chanRequests chan int32,
	chanResponses chan int32,
) hostModuleOption {
	return wazergo.OptionFunc(func(m *hostModuleInstance) {
		m.cubeRequests = chanRequests
		m.cubeResponses = chanResponses
	})
}

// hostModuleInstance is used to maintain the state of our module instance.
type hostModuleInstance struct {
	cubeRequests  chan int32
	cubeResponses chan int32
}

func (*hostModuleInstance) Close(context.Context) error { return nil }

func (m *hostModuleInstance) cubeRequest(ctx context.Context) types.Int32 {
	req := <-m.cubeRequests
	return types.Int32(req)
}

func (m *hostModuleInstance) cubeResponse(ctx context.Context, out types.Int32) {
	m.cubeResponses <- int32(out)
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
