package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/urfave/cli/v2"
)

func NewApp() *cli.App {
	return &cli.App{
		Name:    "load-send",
		Version: "v0.0.1",
		Commands: []*cli.Command{
			{
				Name: "http",
				Action: func(ctx *cli.Context) error {
					vu := ctx.Int("vu")
					duration := ctx.Int("d")
					reqUrl := ctx.String("u")
					reqMethod := strings.ToUpper(ctx.String("m"))
					reqHeaders := ctx.StringSlice("h")
					reqData := ctx.String("b")

					var body io.Reader = nil
					if len(reqData) > 0 {
						fmt.Println("found body", reqData)
						body = bytes.NewBuffer([]byte(reqData))
					}

					req, err := http.NewRequest(reqMethod, reqUrl, body)
					if err != nil {
						return err
					}

					for _, h := range reqHeaders {
						splits := strings.SplitN(h, "=", 1)
						req.Header.Set(splits[0], splits[1])
					}

					return SendRequests(ctx.Context, SendRequestsParams{
						VU:       vu,
						Duration: time.Second * time.Duration(duration),
						Req:      req,
					})
				},
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:        "virual-users",
						Aliases:     []string{"vu"},
						Value:       10,
						DefaultText: "10",
						Usage:       "number of virtual users",
					},
					&cli.IntFlag{
						Name:        "duration",
						Aliases:     []string{"d"},
						Value:       60,
						DefaultText: "60",
						Usage:       "duration to run (in seconds)",
					},
					&cli.StringFlag{
						Name:        "method",
						Aliases:     []string{"m"},
						Value:       "GET",
						DefaultText: "GET",
						Usage:       "http method for request",
					},
					&cli.StringSliceFlag{
						Name:    "header",
						Aliases: []string{"H"},
						Usage:   "request headers",
					},
					&cli.StringFlag{
						Name:    "body",
						Aliases: []string{"b"},
						Usage:   "request data",
					},
					&cli.StringFlag{
						Name:     "url",
						Aliases:  []string{"u"},
						Usage:    "request url",
						Required: true,
					},
				},
			},
		},
	}
}

type SendRequestsParams struct {
	VU       int
	Duration time.Duration
	Req      *http.Request
}

type RequestResult struct {
	Response *http.Response
	Duration time.Duration
}

func SendRequests(ctx context.Context, params SendRequestsParams) error {
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(ctx)
	resultCh := make(chan RequestResult, params.VU)
	var totalRequests int64
	var totalFailedRequests int64
	var totalPassedRequests int64
	var totalTime int64

	// Spin up worker goroutines
	for i := 0; i < params.VU; i++ {
		wg.Add(1)
		go func(ctx context.Context) {
		outerloop:
			for {
				select {
				case <-ctx.Done():
					break outerloop
				default:
					start := time.Now()
					resp, err := http.DefaultClient.Do(params.Req)
					if err != nil {
						if strings.Contains(err.Error(), "request canceled") {
							continue
						}
						fmt.Println(err)
					}
					resp.Body.Close()

					resultCh <- RequestResult{
						Response: resp,
						Duration: time.Since(start),
					}
				}
			}
			wg.Done()
		}(ctx)
	}

	timer := time.NewTimer(params.Duration)
	go func() {
		<-timer.C
		cancel()
	}()

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for res := range resultCh {
		totalTime += res.Duration.Milliseconds()
		totalRequests++
		if res.Response.StatusCode >= 200 && res.Response.StatusCode < 300 {
			totalPassedRequests++
		} else {
			totalFailedRequests++
		}
	}

	fmt.Println("Total Requests:", totalRequests)
	fmt.Println("Total Passed (200-299):", totalPassedRequests)
	fmt.Println("Total Failed (>300):", totalFailedRequests)
	fmt.Println("Total Request Time (ms):", totalTime)
	fmt.Println("Average Latency (ms):", totalTime/totalRequests)

	return nil
}
