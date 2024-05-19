package utils

import (
	"fmt"

	"go.starlark.net/starlark"
)

func NewErrorWithStack(thread *starlark.Thread, err error) error {
	return fmt.Errorf("\n%w\n%v", err, thread.CallStack())
}
