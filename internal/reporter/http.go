package repoter

import (
	"context"
	"fmt"
	"sort"
	"time"
)

func NewHttp() *HttpRepoter {
	return &HttpRepoter{
		collectorCh: make(chan *Record, 100),
		closeCh:     make(chan struct{}),
		records:     make([]*Record, 0),
	}
}

var _ Repoter = (*HttpRepoter)(nil)

type HttpRepoter struct {
	collectorCh chan *Record
	closeCh     chan struct{}
	records     []*Record
}

// NewRecord implements Repoter.
func (h *HttpRepoter) NewRecord() *Record {
	return &Record{
		recordType:    "http",
		data:          make(map[string]any),
		failedReasons: make(map[string]int),
	}
}

func (h *HttpRepoter) Record(record *Record) error {
	h.collectorCh <- record

	return nil
}

func (h *HttpRepoter) Type() string {
	return "http"
}

func (h *HttpRepoter) GetReport() Report {
	var totalRequests int = len(h.records)
	var totalTime time.Duration
	var totalSuccessRequests int
	var totalFailedRequests int

	bucketStart := *h.records[0].start
	for _, record := range h.records {
		if record.start.Before(bucketStart) {
			bucketStart = *record.start
		}
	}

	sort.Slice(h.records, func(i, j int) bool {
		return h.records[i].end.Before(*h.records[j].end)
	})

	bucketCount := 1.0

	for _, record := range h.records {
		if record.end.After(bucketStart.Add(time.Second)) {
			bucketCount++
			bucketStart = bucketStart.Add(time.Second)
		}

		totalTime += record.duration()

		if record.success {
			totalSuccessRequests++
		} else {
			totalFailedRequests++
		}
	}

	return Report{
		DisplayLines: []string{
			fmt.Sprintf("Total Requests: %d", len(h.records)),
			fmt.Sprintf("Total Request Time: %.2f seconds", totalTime.Seconds()),
			fmt.Sprintf("Total Success Requests: %d", totalSuccessRequests),
			fmt.Sprintf("Total Failed Requests: %d", totalFailedRequests),
			fmt.Sprintf("Requests per second: %.2f", float64(totalRequests)/bucketCount),
		},
	}
}

func (h *HttpRepoter) Start(ctx context.Context) error {
	go func() {
		for record := range h.collectorCh {
			h.records = append(h.records, record)
		}

		h.closeCh <- struct{}{}
	}()

	return nil
}

func (h *HttpRepoter) Stop() error {
	close(h.collectorCh)
	<-h.closeCh

	return nil
}
