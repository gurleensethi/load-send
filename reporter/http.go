package reporter

import (
	"fmt"
	"os"
	"sync"
	"text/tabwriter"
)

func NewHttpStatusReporter() *HttpStatusReporter {
	return &HttpStatusReporter{
		failedRequestReasons: make(map[string]int),
		reportCh:             make(chan report, 100),
		wg:                   &sync.WaitGroup{},
	}
}

type report struct {
	reqDuration int
	isSuccess   bool
	failReason  string
}

type HttpStatusReporter struct {
	totalRequests           int
	totalSuccessRequests    int
	totalFailedRequests     int
	totalSuccessRequestTime int
	totalFailedRequestTime  int
	totalRequestTime        int
	failedRequestReasons    map[string]int
	reportCh                chan report
	wg                      *sync.WaitGroup
}

func (r *HttpStatusReporter) Start() {
	r.wg.Add(1)
	go func() {
		for rep := range r.reportCh {
			r.totalRequests++
			r.totalRequestTime += rep.reqDuration

			if rep.isSuccess {
				r.totalSuccessRequests++
			} else {
				r.failedRequestReasons[rep.failReason]++
			}
		}

		r.wg.Done()
	}()
}

func (r *HttpStatusReporter) Stop() {
	close(r.reportCh)
	r.wg.Wait()
}

func (r *HttpStatusReporter) ReportSuccessRequest(duration int) {
	r.totalRequests++
	r.totalSuccessRequests++
	r.totalSuccessRequestTime += duration
	r.totalRequestTime += duration
}

func (r *HttpStatusReporter) ReportFailedRequest(duration int, reason string) {
	r.totalRequests++
	r.totalFailedRequests++
	r.totalFailedRequestTime += duration
	r.totalRequestTime += duration
	r.failedRequestReasons[reason]++
}

func (r *HttpStatusReporter) Print() {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 5, ' ', tabwriter.Debug|tabwriter.AlignRight)

	var averageLatency int
	if r.totalRequests > 0 {
		averageLatency = r.totalRequestTime / r.totalRequests
	}

	var successRPS float64
	if r.totalSuccessRequests > 0 {
		successRPS = float64(r.totalSuccessRequests) / float64(r.totalSuccessRequestTime/1000)
	}

	var failedRPS float64
	if r.totalFailedRequests > 0 {
		failedRPS = float64(r.totalFailedRequests) / float64(r.totalFailedRequestTime/1000)
	}

	var totalRPS float64
	if r.totalRequests > 0 {
		totalRPS = float64(r.totalRequests) / float64(r.totalRequestTime/1000)
	}

	fmt.Fprintf(writer, "\n\n")
	fmt.Fprintf(writer, "Total Requests \t %d\n", r.totalRequests)
	fmt.Fprintf(writer, "Total Success Requests \t %d\n", r.totalSuccessRequests)
	fmt.Fprintf(writer, "Total Failed Requests \t %d\n", r.totalFailedRequests)
	if len(r.failedRequestReasons) > 0 {
		for key, value := range r.failedRequestReasons {
			fmt.Fprintf(writer, "%s \t %d\n", key, value)
		}
	}
	fmt.Fprintf(writer, "Total Success Request Time (ms) \t %d\n", r.totalSuccessRequestTime)
	fmt.Fprintf(writer, "Total Failed Request Time (ms) \t %d\n", r.totalFailedRequestTime)
	fmt.Fprintf(writer, "Total Request Time (ms) \t %d\n", r.totalRequestTime)
	fmt.Fprintf(writer, "Average Latency (ms) \t %d\n", averageLatency)
	fmt.Fprintf(writer, "Success RPS \t %0.2f\n", successRPS)
	fmt.Fprintf(writer, "Failed RPS \t %0.2f\n", failedRPS)
	fmt.Fprintf(writer, "Total RPS \t %0.2f\n", totalRPS)
	_ = writer.Flush()
	fmt.Fprintf(writer, "\n\n")
}
