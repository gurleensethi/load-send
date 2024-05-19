package modules

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	repoter "github.com/gurleensethi/load-send/internal/reporter"
	"github.com/gurleensethi/load-send/internal/starlark/utils"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

type LoadModule struct {
	starlarkstruct.Module
}

type LoadsendReporters struct {
	HttpRepoter *repoter.HttpRepoter
}

func NewLoadSend(repoters LoadsendReporters) *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "loadsend",
		Members: starlark.StringDict{
			"http": starlark.NewBuiltin("http", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				var method string = "GET"
				var url string
				var body string
				var headers *starlark.Dict

				err := starlark.UnpackArgs(fn.Name(), args, kwargs,
					"method?", &method,
					"url", &url,
					"body?", &body,
					"headers?", &headers,
				)
				if err != nil {
					return nil, utils.NewErrorWithStack(thread, err)
				}

				if url == "" {
					return nil, utils.NewErrorWithStack(thread, errors.New("url cannot be empty"))
				}

				var b io.Reader
				if body != "" {
					b = bytes.NewBuffer([]byte(body))
				}

				httpReq, err := http.NewRequest(method, url, b)
				if err != nil {
					return nil, utils.NewErrorWithStack(thread, err)
				}

				record := repoters.HttpRepoter.NewRecord()
				record.Set("url", url)
				record.Set("method", method)

				record.Start()
				resp, err := http.DefaultClient.Do(httpReq)
				if err != nil {
					return nil, utils.NewErrorWithStack(thread, err)
				}
				record.End()

				respBody, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, utils.NewErrorWithStack(thread, err)
				}

				err = resp.Body.Close()
				if err != nil {
					return nil, utils.NewErrorWithStack(thread, err)
				}

				respHeaders := starlark.NewDict(0)
				for key, value := range httpReq.Header {
					err := respHeaders.SetKey(starlark.String(key), starlark.String(strings.Join(value, ";")))
					if err != nil {
						return nil, err
					}
				}

				err = repoters.HttpRepoter.Record(record)
				if err != nil {
					return nil, utils.NewErrorWithStack(thread, err)
				}

				return starlarkstruct.FromStringDict(starlark.String("http_response"), starlark.StringDict{
					"status_code":    starlark.MakeInt(resp.StatusCode),
					"status":         starlark.String(resp.Status),
					"body":           starlark.String(respBody),
					"headers":        respHeaders,
					"content_length": starlark.MakeInt64(resp.ContentLength),
					"success": starlark.NewBuiltin("success", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
						record.Success()

						return starlark.None, nil
					}),
					"error": starlark.NewBuiltin("error", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
						var reason starlark.String = "<no reason>"

						err := starlark.UnpackArgs(fn.Name(), args, kwargs,
							"reason?",
							&reason,
						)
						if err != nil {
							return nil, utils.NewErrorWithStack(thread, err)
						}

						record.Failed(reason.GoString())

						return starlark.None, nil
					}),
				}), nil
			}),
		},
	}
}
