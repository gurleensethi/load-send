package script

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

type LifecycleScript struct{}

func New() *LifecycleScript {
	return &LifecycleScript{}
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

	thread := &starlark.Thread{Name: "main"}
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, filename, nil, starlark.StringDict{})
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
	if lc.beforeAllFn != nil {
		_, err := starlark.Call(thread, lc.beforeAllFn, starlark.Tuple{}, []starlark.Tuple{})
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

			args := starlark.Tuple{
				starlark.MakeInt(num),
			}

		outerloop:
			for {
				select {
				case <-ctx.Done():
					break outerloop
				default:
					_, err := starlark.Call(thread, lc.runFn, args, []starlark.Tuple{})
					if err != nil {
						fmt.Println(err)
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
