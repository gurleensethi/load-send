package script

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

type LifecycleScript struct {
	Modules map[string]*starlarkstruct.Module
}

func New(m map[string]*starlarkstruct.Module) *LifecycleScript {
	return &LifecycleScript{
		Modules: m,
	}
}

type RunOptions struct {
	VU       int
	Duration time.Duration
}

func (s *LifecycleScript) Run(ctx context.Context, filename string, opts *RunOptions) error {
	if opts == nil {
		opts = &RunOptions{
			VU:       5,
			Duration: 5 * time.Second,
		}
	}

	predeclared := starlark.StringDict{}
	if s.Modules != nil {
		for key, value := range s.Modules {
			predeclared[key] = value
		}
	}

	thread := &starlark.Thread{Name: "main"}
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
	}, thread, filename, nil, predeclared)
	if err != nil {
		return err
	}

	runFn, err := getGlobalFunction(globals, "run", true)
	if err != nil {
		return err
	}

	beforeAllFn, err := getGlobalFunction(globals, "before_all", false)
	if err != nil {
		return err
	}

	afterAllFn, err := getGlobalFunction(globals, "after_all", false)
	if err != nil {
		return err
	}

	beforeEachFn, err := getGlobalFunction(globals, "before_each", false)
	if err != nil {
		return err
	}

	afterEachFn, err := getGlobalFunction(globals, "after_each", false)
	if err != nil {
		return err
	}

	lc := lifecycle{
		beforeAllFn:  beforeAllFn,
		beforeEachFn: beforeEachFn,
		runFn:        runFn,
		afterEachFn:  afterEachFn,
		afterAllFn:   afterAllFn,
	}

	return s.runLifecycle(ctx, thread, lc, lifecycleOpts{
		iterations:    opts.VU,
		timeout:       opts.Duration,
		strictTimeout: false,
	})
}

type lifecycle struct {
	beforeAllFn  *starlark.Function
	beforeEachFn *starlark.Function
	runFn        *starlark.Function
	afterEachFn  *starlark.Function
	afterAllFn   *starlark.Function
}

type lifecycleOpts struct {
	iterations    int
	timeout       time.Duration
	strictTimeout bool
}

func (s *LifecycleScript) runLifecycle(ctx context.Context, thread *starlark.Thread, lc lifecycle, opts lifecycleOpts) error {
	var beforeAllReturn starlark.Value

	if lc.beforeAllFn != nil {
		var err error
		beforeAllReturn, err = starlark.Call(thread, lc.beforeAllFn, nil, nil)
		if err != nil {
			return err
		}
	}

	workersWG := &sync.WaitGroup{}

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)

	// Spin up workers that mimic VUs
	// TODO: add graceful spinup
	for i := 0; i < opts.iterations; i++ {
		workersWG.Add(1)
		go func(ctx context.Context, num int) {
			thread := &starlark.Thread{Name: fmt.Sprintf("runner: %d", num)}

		outerloop:
			for {
				select {
				case <-ctx.Done():
					break outerloop
				default:
					var beforeEachReturn starlark.Value
					var runReturn starlark.Value
					var err error

					// ===== Before Each =====

					if lc.beforeEachFn != nil {
						beforeEachArgs := starlark.Tuple{}
						if lc.beforeEachFn.NumParams() > 0 {
							data := starlarkstruct.FromKeywords(starlarkstruct.Default, []starlark.Tuple{
								{starlark.String("before_all"), beforeAllReturn},
							})
							beforeEachArgs = append(beforeEachArgs, data)
						}

						beforeEachReturn, err = starlark.Call(thread, lc.beforeEachFn, beforeEachArgs, []starlark.Tuple{})
						if err != nil {
							fmt.Println(err)
							break
						}
					}

					// ===== Run =====

					runArgs := starlark.Tuple{}
					if lc.runFn.NumParams() > 0 {
						data := starlarkstruct.FromKeywords(starlarkstruct.Default, []starlark.Tuple{
							{starlark.String("before_all"), beforeAllReturn},
							{starlark.String("before_each"), beforeEachReturn},
						})
						runArgs = append(runArgs, data)
					}

					runReturn, err = starlark.Call(thread, lc.runFn, runArgs, []starlark.Tuple{})
					if err != nil {
						fmt.Println(err)
						break
					}

					// ===== After Each =====

					afterEachArgs := starlark.Tuple{}
					if lc.afterEachFn.NumParams() > 0 {
						data := starlarkstruct.FromKeywords(starlarkstruct.Default, []starlark.Tuple{
							{starlark.String("before_all"), beforeAllReturn},
							{starlark.String("before_each"), beforeEachReturn},
							{starlark.String("run"), runReturn},
						})
						afterEachArgs = append(afterEachArgs, data)
					}

					if lc.afterEachFn != nil {
						_, err := starlark.Call(thread, lc.afterEachFn, afterEachArgs, []starlark.Tuple{})
						if err != nil {
							fmt.Println(err)
							break
						}
					}
				}

				time.Sleep(time.Second)
			}

			workersWG.Done()
		}(ctx, i)
	}

	workersWG.Wait()
	cancel()

	if lc.afterAllFn != nil {
		_, err := starlark.Call(thread, lc.afterAllFn, starlark.Tuple{}, []starlark.Tuple{})
		if err != nil {
			return err
		}
	}

	return nil
}

func getGlobalFunction(d starlark.StringDict, f string, required bool) (*starlark.Function, error) {
	if !d.Has(f) {
		if required {
			return nil, errors.New(f + "() function not defined in script")
		}

		return nil, nil
	}

	fn := d[f]

	if fn.Type() != "function" {
		return nil, errors.New(f + " should be a function, but found: " + fn.Type())
	}

	return fn.(*starlark.Function), nil
}
