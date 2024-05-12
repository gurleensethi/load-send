package script

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gurleensethi/load-send/reporter"
	"github.com/gurleensethi/load-send/script/modules/load"
	"github.com/risor-io/risor"
	"github.com/risor-io/risor/compiler"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/parser"
	"github.com/risor-io/risor/vm"
)

type RunLoadScriptOptions struct {
	Duration time.Duration
	Verbose  bool
	VU       int
}

func RunLoadScript(ctx context.Context, script string, opts RunLoadScriptOptions) error {
	httpReporter := reporter.NewHttpStatusReporter()
	httpReporter.Start()

	risorCfg := risor.NewConfig(
		risor.WithGlobals(map[string]any{
			"load": load.Module(httpReporter),
		}),
	)

	ast, err := parser.Parse(ctx, script)
	if err != nil {
		return err
	}

	main, err := compiler.Compile(ast, risorCfg.CompilerOpts()...)
	if err != nil {
		return err
	}

	vm := vm.New(main, risorCfg.VMOpts()...)

	err = vm.Run(ctx)
	if err != nil {
		return err
	}

	// TODO: Look for run function before executing the script
	runFn, err := getFunFromVM(ctx, vm, "run")
	if err != nil {
		return err
	}
	if runFn == nil {
		return errors.New("run() function not defined in script")
	}

	beforeFn, err := getFunFromVM(ctx, vm, "before")
	if err != nil {
		return err
	}

	afterFn, err := getFunFromVM(ctx, vm, "after")
	if err != nil {
		return err
	}

	err = executeScriptLifecycle(ctx, vm, scriptLifeCycleOpts{
		beforeFn: beforeFn,
		runFn:    runFn,
		afterFn:  afterFn,
		opts:     opts,
	})
	if err != nil {
		return err
	}

	httpReporter.Stop()

	return nil
}

func getFunFromVM(_ context.Context, vm *vm.VirtualMachine, fnName string) (*object.Function, error) {
	fnObject, err := vm.Get(fnName)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return nil, err
	}

	fn, ok := fnObject.(*object.Function)
	if !ok {
		return nil, nil
	}

	return fn, nil
}

type scriptLifeCycleOpts struct {
	beforeFn *object.Function
	runFn    *object.Function
	afterFn  *object.Function
	opts     RunLoadScriptOptions
}

func executeScriptLifecycle(ctx context.Context, virtualMachine *vm.VirtualMachine, o scriptLifeCycleOpts) error {
	if o.beforeFn != nil {
		_, err := virtualMachine.Call(ctx, o.beforeFn, []object.Object{})
		if err != nil {
			return err
		}
	}

	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, o.opts.VU)

	// Spin up workers
	for i := 0; i < o.opts.VU; i++ {
		// Clone the vm so that we can do parallel executions of the runFn.
		clonedVM, err := virtualMachine.Clone()
		if err != nil {
			cancel()
			return err
		}

		wg.Add(1)

		go func(ctx context.Context, virtualMachine *vm.VirtualMachine) {
		outerloop:
			for {
				select {
				case <-ctx.Done():
					break outerloop
				default:
					_, err = virtualMachine.Call(context.WithoutCancel(ctx), o.runFn, []object.Object{})
					if err != nil {
						errCh <- err
						break outerloop
					}
				}
			}

			wg.Done()
		}(ctx, clonedVM)
	}

	var runErr error
	// Look for any errors and close script as soon as an error occurs
	go func() {
		select {
		// Catch the first error
		case runErr = <-errCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Timer to cancel workers after duration
	timer := time.NewTimer(o.opts.Duration * time.Second)
	go func() {
		<-timer.C
		cancel()
	}()

	// Wait for all workers to stop
	wg.Wait()

	if runErr != nil {
		return runErr
	}

	if o.afterFn != nil {
		_, err := virtualMachine.Call(ctx, o.afterFn, []object.Object{})
		if err != nil {
			return err
		}
	}

	return nil
}
