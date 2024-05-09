package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"
)

func NewApp() *cli.App {
	return &cli.App{
		Name:    "load-send",
		Version: "v0.0.4",
		Commands: []*cli.Command{
			{
				Name: "http",
				Action: func(ctx *cli.Context) error {
					vu := ctx.Int("virual-users")
					duration := ctx.Int("duration")
					reqUrl := ctx.String("url")
					reqMethod := strings.ToUpper(ctx.String("method"))
					reqData := ctx.String("body")
					reqTimeout := ctx.Int("timeout")
					verbose := ctx.Bool("verbose")

					rawHeaders := ctx.StringSlice("header")
					reqHeaders := make(map[string]string)
					for _, h := range rawHeaders {
						splits := strings.SplitN(h, ":", 2)
						key := splits[0]
						value := ""
						if len(splits) > 1 {
							value = splits[1]
						}
						reqHeaders[key] = value
					}

					return SendRequests(ctx.Context, SendRequestsParams{
						VU:         vu,
						Duration:   time.Second * time.Duration(duration),
						ReqUrl:     reqUrl,
						ReqMethod:  reqMethod,
						ReqHeaders: reqHeaders,
						ReqData:    reqData,
						ReqTimeout: reqTimeout,
						Verbose:    verbose,
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
					&cli.IntFlag{
						Name:    "timeout",
						Aliases: []string{"to"},
						Value:   30,
						Usage:   "request timeout",
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "verbose mode",
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
		totalTime += res.Duration.Milliseconds()
		totalRequests++
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
		responseStatusCodeCount: responseStatusCodeCount,
	})

	return nil
}

type result struct {
	totalRequests           int64
	totalFailedRequests     int64
	totalPassedRequests     int64
	totalTime               int64
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

	fmt.Fprintf(writer, "\n\n")
	fmt.Fprintf(writer, "VUs \t %d\n", params.VU)
	fmt.Fprintf(writer, "Duration \t %s\n", params.Duration.String())
	fmt.Fprintf(writer, "Total Requests \t %d\n", r.totalRequests)
	for _, line := range statusCodeLines {
		fmt.Fprint(writer, line)
	}
	fmt.Fprintf(writer, "Total Request Time (ms) \t %d\n", r.totalTime)
	fmt.Fprintf(writer, "Average Latency (ms) \t %d\n", r.totalTime/r.totalRequests)
	fmt.Fprintf(writer, "requests/sec \t %d\n", r.totalRequests/int64(params.Duration.Seconds()))
	_ = writer.Flush()
	fmt.Fprintf(writer, "\n\n")
}
