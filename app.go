package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/gurleensethi/load-send/internal/starlark/modules/loadsend"
	lifecyclescript "github.com/gurleensethi/load-send/internal/starlark/script"
	"github.com/gurleensethi/load-send/script"
	"github.com/urfave/cli/v2"
	"go.starlark.net/starlarkstruct"
)

const (
	loadScriptNotFoundMsg = "load script not provided\nusage: load-send <path_to_script>"
)

func NewApp() *cli.App {
	return &cli.App{
		Name:    "load-send",
		Version: "v0.0.4",
		Action: func(ctx *cli.Context) error {
			s := lifecyclescript.New(map[string]*starlarkstruct.Module{
				"loadsend": loadsend.New(),
			})
			return s.Run(ctx.Context, ctx.Args().Get(0), nil)
		},
		Commands: []*cli.Command{
			{
				Name: "http",
				Action: func(ctx *cli.Context) error {
					if ctx.NArg() == 0 {
						return errors.New(loadScriptNotFoundMsg)
					}

					scriptFile, err := os.ReadFile(ctx.Args().First())
					if err != nil {
						return err
					}

					duration := ctx.Int("duration")
					verbose := ctx.Bool("verbose")
					vu := ctx.Int("virual-users")

					err = script.RunLoadScript(ctx.Context, string(scriptFile), script.RunLoadScriptOptions{
						Duration: time.Duration(duration),
						Verbose:  verbose,
						VU:       vu,
					})
					if err != nil {
						return err
					}

					return nil
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
					&cli.BoolFlag{
						Name:  "verbose",
						Usage: "verbose mode",
					},
				},
			},
		},
	}
}

type SendRequestsParams struct {
	Duration   time.Duration
	ReqData    string
	ReqHeaders map[string]string
	ReqMethod  string
	ReqUrl     string
	ReqTimeout int
	Verbose    bool
	VU         int
}

type RequestResult struct {
	Response *http.Response
	Duration time.Duration
	Timeout  bool
}

func SendRequests(ctx context.Context, params SendRequestsParams) error {
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(ctx)
	resultCh := make(chan RequestResult, params.VU)
	httpClient := http.Client{
		Timeout: time.Second * time.Duration(params.ReqTimeout),
	}
	var totalRequests int64
	var totalFailedRequests int64
	var totalPassedRequests int64
	var totalTime int64
	var totalTimeouts int64
	var responseStatusCodeCount = make(map[int]int)

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
					var body io.Reader = nil
					if len(params.ReqData) > 0 {
						body = bytes.NewBuffer([]byte(params.ReqData))
					}

					req, err := http.NewRequest(params.ReqMethod, params.ReqUrl, body)
					if err != nil {
						panic(err)
					}

					for key, value := range params.ReqHeaders {
						req.Header.Set(key, value)
					}

					start := time.Now()
					resp, err := httpClient.Do(req)
					if err != nil {
						if strings.Contains(err.Error(), "request canceled") {
							continue
						}

						var urlErr *url.Error
						if errors.As(err, &urlErr) && urlErr.Timeout() {
							resultCh <- RequestResult{
								Timeout: true,
							}
							continue
						}

						fmt.Println(err)
					}

					if params.Verbose {
						fmt.Println("Response Body:")
						b, _ := io.ReadAll(resp.Body)
						fmt.Println(string(b))
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

	ticker := time.NewTicker(time.Millisecond * 200)
	timer := time.NewTimer(params.Duration)
	start := time.Now()
	go func() {
		<-timer.C
		ticker.Stop()
		cancel()
	}()

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	go func() {
		for {
			_, ok := <-ticker.C
			if !ok {
				break
			}

			progress := (time.Since(start).Milliseconds() * 100 / params.Duration.Milliseconds())
			// remaining := 100 - progress

			totalBars := 50
			progressBars := (totalBars * int(progress)) / 100
			remainingBars := totalBars - progressBars

			fmt.Printf("\rSending Requests [%s>%s] %d%%",
				strings.Repeat("=", int(progressBars)),
				strings.Repeat("-", int(remainingBars)),
				progress,
			)
		}
	}()

	for res := range resultCh {
		if res.Timeout {
			totalTimeouts++
			continue
		}

		totalRequests++
		totalTime += res.Duration.Milliseconds()
		if res.Response.StatusCode >= 200 && res.Response.StatusCode < 300 {
			totalPassedRequests++
		} else {
			totalFailedRequests++
		}

		currentCount := responseStatusCodeCount[res.Response.StatusCode]
		currentCount++
		responseStatusCodeCount[res.Response.StatusCode] = currentCount
	}

	printResult(params, result{
		totalRequests:           totalRequests,
		totalFailedRequests:     totalFailedRequests,
		totalPassedRequests:     totalPassedRequests,
		totalTime:               totalTime,
		totalTimeouts:           totalTimeouts,
		responseStatusCodeCount: responseStatusCodeCount,
	})

	return nil
}

type result struct {
	totalRequests           int64
	totalFailedRequests     int64
	totalPassedRequests     int64
	totalTime               int64
	totalTimeouts           int64
	responseStatusCodeCount map[int]int
}

func printResult(params SendRequestsParams, r result) {
	statusCodeLines := make([]string, 0)
	for key, value := range r.responseStatusCodeCount {
		statusCodeLines = append(statusCodeLines,
			fmt.Sprintf("    HTTP %d \t %d\n", key, value),
		)
	}
	slices.Sort(statusCodeLines)

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 5, ' ', tabwriter.Debug|tabwriter.AlignRight)

	var averageLatency int64
	if r.totalRequests > 0 {
		averageLatency = r.totalTime / r.totalRequests
	}

	fmt.Fprintf(writer, "\n\n")
	fmt.Fprintf(writer, "VUs \t %d\n", params.VU)
	fmt.Fprintf(writer, "Duration \t %s\n", params.Duration.String())
	fmt.Fprintf(writer, "Total Requests \t %d\n", r.totalRequests)
	for _, line := range statusCodeLines {
		fmt.Fprint(writer, line)
	}
	fmt.Fprintf(writer, "Total Timeouts \t %d\n", r.totalTimeouts)
	fmt.Fprintf(writer, "Total Request Time (ms) \t %d\n", r.totalTime)
	fmt.Fprintf(writer, "Average Latency (ms) \t %d\n", averageLatency)
	fmt.Fprintf(writer, "requests/sec \t %d\n", r.totalRequests/int64(params.Duration.Seconds()))
	_ = writer.Flush()
	fmt.Fprintf(writer, "\n\n")
}
