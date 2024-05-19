package repoter

import (
	"context"
	"time"
)

type Repoter interface {
	Start(ctx context.Context) error
	NewRecord() *Record
	Record(record *Record) error
	GetReport() Report
	Stop() error
	Type() string
}

type Report struct {
	DisplayLines []string
}

type Record struct {
	recordType    string
	start         *time.Time
	end           *time.Time
	success       bool
	data          map[string]any
	failedReasons map[string]int
}

func (r *Record) Start() {
	now := time.Now()
	r.start = &now
}

func (r *Record) End() {
	now := time.Now()
	r.end = &now
}

func (r *Record) Set(key string, val any) {
	r.data[key] = val
}

func (r *Record) Del(key string) {
	delete(r.data, key)
}

func (r *Record) Success() {
	r.success = true
}

func (r *Record) Failed(reason string) {
	r.success = false
}

func (r *Record) Clear() {
	clear(r.data)
}

func (r *Record) duration() time.Duration {
	if r.start == nil || r.end == nil {
		panic("record should have a start and end timestamp to get duration")
	}

	return r.end.Sub(*r.start)
}
