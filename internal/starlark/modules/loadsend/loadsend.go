package loadsend

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

type LoadModule struct {
	starlarkstruct.Module
}

func New() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "loadsend",
		Members: starlark.StringDict{
			"http": starlark.NewBuiltin("http", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				fmt.Println("calling http module")
				return starlark.None, nil
			}),
		},
	}
}
