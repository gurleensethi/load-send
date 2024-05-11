package load

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/risor-io/risor/object"
)

func Module() *object.Module {
	return object.NewBuiltinsModule("load", map[string]object.Object{
		"http": object.NewBuiltin("http", httpFn),
	})
}

func httpFn(ctx context.Context, args ...object.Object) object.Object {
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
	fmt.Println(httpResp, httpErr)
	end := time.Since(start)

	fmt.Println(end)

	return object.Nil
}
