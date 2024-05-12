package reporter

import (
	"fmt"
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func NewHttpStatusReporter() *HttpStatusReporter {
	return &HttpStatusReporter{
		failedRequestReasons: make(map[string]int),
		reportCh:             make(chan report, 100),
		closeTUIUpdatesCh:    make(chan struct{}),
		closeUICh:            make(chan struct{}),
		closeReportCh:        make(chan struct{}),
	}
}

type report struct {
	reqDuration int
	isSuccess   bool
	failReason  string
}

type HttpStatusReporter struct {
	startTime               time.Time
	totalRequests           int
	totalSuccessRequests    int
	totalFailedRequests     int
	totalSuccessRequestTime int
	totalFailedRequestTime  int
	totalRequestTime        int
	failedRequestReasons    map[string]int
	reportCh                chan report
	closeTUIUpdatesCh       chan struct{}
	closeUICh               chan struct{}
	closeReportCh           chan struct{}
	teaProgram              *tea.Program
}

func (r *HttpStatusReporter) Start() {
	// Start the UI
	go func() {
		r.teaProgram = tea.NewProgram(httpReporterUIModel{})
		_, err := r.teaProgram.Run()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		// Print out the result
		// fmt.Println(model.View())

		r.closeUICh <- struct{}{}
	}()

	go func() {
		for {
			select {
			case <-r.closeTUIUpdatesCh:
				return
			default:
				if r.teaProgram != nil {
					// Send updates to UI
					r.teaProgram.Send(r.GetResult())
				}
			}

			time.Sleep(time.Second)
		}
	}()

	// Start listening to results
	go func() {
		for rep := range r.reportCh {
			r.totalRequests++
			r.totalRequestTime += rep.reqDuration

			if rep.isSuccess {
				r.totalSuccessRequestTime += rep.reqDuration
				r.totalSuccessRequests++
			} else {
				r.totalFailedRequestTime += rep.reqDuration
				r.totalFailedRequests++
				r.failedRequestReasons[rep.failReason]++
			}
		}

		r.closeReportCh <- struct{}{}
	}()

	r.startTime = time.Now()
}

func (r *HttpStatusReporter) Stop() {
	// Close the report channel and wait for anything in
	// the channel buffer to be exhausted.
	close(r.reportCh)
	<-r.closeReportCh

	// Stop the UI updates
	r.closeTUIUpdatesCh <- struct{}{}

	// Quit the UI and wait for the tea.Program
	// to complete
	r.teaProgram.Send(tea.Quit())
	<-r.closeUICh
}

func (r *HttpStatusReporter) ReportSuccessRequest(duration int) {
	r.reportCh <- report{
		reqDuration: duration,
		isSuccess:   true,
	}
}

func (r *HttpStatusReporter) ReportFailedRequest(duration int, reason string) {
	r.reportCh <- report{
		reqDuration: duration,
		isSuccess:   false,
		failReason:  reason,
	}
}

type HttpReporterResult struct {
	AverageFailedLatency    int
	AverageLatency          int
	AverageSuccessLatency   int
	FailedRPS               float64
	SuccessRPS              float64
	TotalFailedRequests     int
	TotalFailedRequestTime  int
	FailedRequestReasons    map[string]int
	TotalRequests           int
	TotalRequestTimeMillis  int
	TotalRPS                float64
	TotalSuccessRequests    int
	TotalSuccessRequestTime int
}

func (r *HttpStatusReporter) GetResult() HttpReporterResult {
	elapsedDurationSeconds := time.Since(r.startTime).Seconds()

	var averageLatency int
	if r.totalRequests > 0 {
		averageLatency = r.totalRequestTime / r.totalRequests
	}

	var averageSuccessLatency int
	if r.totalSuccessRequests > 0 {
		averageSuccessLatency = r.totalSuccessRequestTime / r.totalSuccessRequests
	}

	var averageFailedLatency int
	if r.totalFailedRequests > 0 {
		averageFailedLatency = r.totalFailedRequestTime / r.totalFailedRequests
	}

	var successRPS float64
	if r.totalSuccessRequests > 0 {
		successRPS = float64(r.totalSuccessRequests) / elapsedDurationSeconds
	}

	var failedRPS float64
	if r.totalFailedRequests > 0 {
		failedRPS = float64(r.totalFailedRequests) / elapsedDurationSeconds
	}

	var totalRPS float64
	if r.totalRequests > 0 {
		totalRPS = float64(r.totalRequests) / elapsedDurationSeconds
	}

	failedRequestReasons := make(map[string]int)
	for key, value := range r.failedRequestReasons {
		failedRequestReasons[key] = value
	}

	return HttpReporterResult{
		AverageFailedLatency:    averageFailedLatency,
		AverageLatency:          averageLatency,
		AverageSuccessLatency:   averageSuccessLatency,
		FailedRPS:               failedRPS,
		SuccessRPS:              successRPS,
		TotalFailedRequests:     r.totalFailedRequests,
		TotalFailedRequestTime:  r.totalFailedRequestTime,
		TotalRequests:           r.totalRequests,
		FailedRequestReasons:    failedRequestReasons,
		TotalRequestTimeMillis:  r.totalRequestTime,
		TotalRPS:                totalRPS,
		TotalSuccessRequests:    r.totalSuccessRequests,
		TotalSuccessRequestTime: r.totalSuccessRequestTime,
	}
}

type httpReporterUIModel struct {
	httpResult HttpReporterResult
}

func (h httpReporterUIModel) Init() tea.Cmd {
	return nil
}

func (h httpReporterUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case HttpReporterResult:
		h.httpResult = msg
		return h, nil
	}

	return h, nil
}

func (h httpReporterUIModel) View() string {
	box := lipgloss.NewStyle().Margin(2).Padding(2)

	rows := [][]string{}

	rows = append(rows,
		[]string{"Total Requests", strconv.FormatInt(int64(h.httpResult.TotalRequests), 10)},
		[]string{"Total Success Requests", strconv.FormatInt(int64(h.httpResult.TotalSuccessRequests), 10)},
		[]string{"Total Failed Requests", strconv.FormatInt(int64(h.httpResult.TotalFailedRequests), 10)},
	)

	for key, value := range h.httpResult.FailedRequestReasons {
		rows = append(rows, []string{"Failed / " + key, strconv.FormatInt(int64(value), 10)})
	}

	rows = append(rows,
		[]string{"Total Request Time (ms)", strconv.FormatInt(int64(h.httpResult.TotalRequestTimeMillis), 10)},
		[]string{"Average Latency (ms)", strconv.FormatInt(int64(h.httpResult.AverageLatency), 10)},
		[]string{"Average Success Latency (ms)", strconv.FormatInt(int64(h.httpResult.AverageSuccessLatency), 10)},
		[]string{"Average Failed Latency (ms)", strconv.FormatInt(int64(h.httpResult.AverageFailedLatency), 10)},
		[]string{"RPS", strconv.FormatInt(int64(h.httpResult.TotalRPS), 10)},
		[]string{"Success RPS", strconv.FormatInt(int64(h.httpResult.SuccessRPS), 10)},
		[]string{"Failed RPS", strconv.FormatInt(int64(h.httpResult.FailedRPS), 10)},
	)

	t := table.New().
		Headers("Metric", "Value").
		Width(50).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row%2 == 1 {
				return lipgloss.NewStyle().PaddingLeft(2).PaddingRight(2)
			} else {
				return lipgloss.NewStyle().PaddingLeft(2).PaddingRight(2)
			}
		}).
		Rows(rows...)

	width, _ := lipgloss.Size(t.Render())

	title := lipgloss.NewStyle().
		Background(lipgloss.Color("99")).
		Foreground(lipgloss.Color("#FAFAFA")).
		AlignHorizontal(lipgloss.Center).
		MarginBottom(1).
		Width(width)

	return box.Render(lipgloss.JoinVertical(lipgloss.Left,
		title.Render("Load Send"),
		t.Render(),
	))
}

var _ tea.Model = httpReporterUIModel{}
