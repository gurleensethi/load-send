package load

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gurleensethi/load-send/reporter"
	"github.com/risor-io/risor/object"
)

func Module(httpReporter *reporter.HttpStatusReporter) *object.Module {
	return object.NewBuiltinsModule("load", map[string]object.Object{
		"http": object.NewBuiltin("http", httpFn(httpReporter)),
	})
}

func httpFn(httpReporter *reporter.HttpStatusReporter) object.BuiltinFunction {
	return func(ctx context.Context, args ...object.Object) object.Object {
		if len(args) != 1 {
			return object.Errorf("`load.http` should be called with exactly 1 argument!")
		}

		optionsMap, err := object.AsMap(args[0])
		if err != nil {
			return object.Errorf("invliad type of argument provided to `load.http`: %v", err)
		}

		reqUrlObj := optionsMap.Get("url")
		reqUrl, err := object.AsString(reqUrlObj)
		if err != nil {
			return object.Errorf("`load.http[url]` should be a valid url: %v", err)
		}

		parsedUrl, urlErr := url.ParseRequestURI(reqUrl)
		if urlErr != nil {
			return object.Errorf("`load.http[url]` should be a valid url: %v", urlErr)
		}

		reqMethodObj := optionsMap.GetWithDefault("method", object.NewString(http.MethodGet))
		reqMethod, err := object.AsString(reqMethodObj)
		if err != nil {
			return object.Errorf("`load.http[method]` should be a valid http method: %v", err)
		}

		reqBodyObj := optionsMap.GetWithDefault("body", object.NewString(""))
		reqBody, err := object.AsString(reqBodyObj)
		if err != nil {
			return object.Errorf("`load.http[body]` should be a raw string: %v", err)
		}

		reqHeadersObj := optionsMap.GetWithDefault("headers", object.NewMap(map[string]object.Object{}))
		reqHeaders, err := object.AsMap(reqHeadersObj)
		if err != nil {
			return object.Errorf("`load.http[headers]` should be a valid map of key=value paris: %v", err)
		}

		var body io.Reader
		if len(reqBody) > 0 {
			body = bytes.NewBuffer([]byte(reqBody))
		}

		httpReq, httpReqErr := http.NewRequest(reqMethod, parsedUrl.String(), body)
		if httpReqErr != nil {
			return object.Errorf("`load.http` failed to create new http request: %v", httpReqErr)
		}

		headerItr := reqHeaders.Iter()
		for {
			_, ok := headerItr.Next(ctx)
			if !ok {
				break
			}

			entry, ok := headerItr.Entry()
			if !ok {
				break
			}

			httpReq.Header.Set(entry.Key().Inspect(), entry.Value().Inspect())
		}

		start := time.Now()
		httpResp, httpErr := http.DefaultClient.Do(httpReq)
		end := time.Since(start)
		if httpErr != nil {
			var urlErr *url.Error
			if errors.As(httpErr, &urlErr) && urlErr.Timeout() {
				httpReporter.ReportFailedRequest(int(end.Milliseconds()), "timeout")
				return object.Nil
			}

			fmt.Println(httpErr)
			return object.Nil
		}

		headers := object.NewMap(map[string]object.Object{})
		for key, value := range httpResp.Header {
			headers.Set(key, object.NewString(strings.Join(value, ";")))
		}

		respBody, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			fmt.Println(readErr)
		}

		return object.NewMap(map[string]object.Object{
			"statusCode":    object.NewInt(int64(httpResp.StatusCode)),
			"status":        object.NewString(httpResp.Status),
			"contentLength": object.NewInt(httpResp.ContentLength),
			"body":          object.NewString(string(respBody)),
			"headers":       headers,
			"success": object.NewBuiltin("success", func(ctx context.Context, args ...object.Object) object.Object {
				httpReporter.ReportSuccessRequest(int(end.Milliseconds()))
				return object.Nil
			}),
			"fail": object.NewBuiltin("error", func(ctx context.Context, args ...object.Object) object.Object {
				reason := "<no reason>"
				if len(args) > 0 {
					reason = args[0].Inspect()
				}

				httpReporter.ReportFailedRequest(int(end.Milliseconds()), reason)
				return object.Nil
			}),
		})
	}
}
