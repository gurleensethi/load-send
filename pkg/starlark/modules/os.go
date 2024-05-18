package modules

import (
	"os"

	"github.com/gurleensethi/load-send/internal/starlark/utils"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var OS = &starlarkstruct.Module{
	Name: "os",
	Members: starlark.StringDict{
		"getenv": starlark.NewBuiltin("getenv", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var key starlark.String

			err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &key)
			if err != nil {
				return nil, utils.NewErrorWithStack(thread, err)
			}

			value := os.Getenv(key.GoString())

			return starlark.String(value), nil
		}),
		"setenv": starlark.NewBuiltin("getenv", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var key starlark.String
			var value starlark.String

			err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &key, &value)
			if err != nil {
				return nil, utils.NewErrorWithStack(thread, err)
			}

			err = os.Setenv(key.GoString(), value.GoString())
			if err != nil {
				return nil, utils.NewErrorWithStack(thread, err)
			}

			return starlark.String(value), nil
		}),
	},
}
