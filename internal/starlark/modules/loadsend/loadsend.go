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
				var method starlark.String
				var url starlark.String
				var body starlark.String
				var headers *starlark.Dict

				err := starlark.UnpackArgs(fn.Name(), args, kwargs,
					"method?", &method,
					"url?", &url,
					"body?", &body,
					"headers?", &headers,
				)
				if err != nil {
					return starlark.None, nil
				}

				fmt.Println(method, url, body, headers)

				return starlark.None, nil
			}),
		},
	}
}
