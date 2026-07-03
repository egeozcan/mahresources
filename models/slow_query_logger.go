package models

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm/logger"
)

// SlowQueryEntry describes a query that exceeded the configured slow-query threshold.
type SlowQueryEntry struct {
	SQL     string
	Elapsed time.Duration
	Rows    int64
	Source  string
}

// SlowQueryLogger wraps a GORM logger.Interface and additionally forwards
// queries slower than the threshold to a sink. The sink is set after the
// database connection is created (the application context that owns it is
// constructed later), so it lives behind an atomic pointer shared across
// LogMode copies.
type SlowQueryLogger struct {
	wrapped   logger.Interface
	threshold time.Duration
	sink      *atomic.Pointer[func(SlowQueryEntry)]
}

func newSlowQueryLogger(wrapped logger.Interface, threshold time.Duration) *SlowQueryLogger {
	return &SlowQueryLogger{
		wrapped:   wrapped,
		threshold: threshold,
		sink:      &atomic.Pointer[func(SlowQueryEntry)]{},
	}
}

// SetSink registers the receiver for slow queries. The sink runs on the
// query's own goroutine and must not block.
func (l *SlowQueryLogger) SetSink(fn func(SlowQueryEntry)) {
	l.sink.Store(&fn)
}

func (l *SlowQueryLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &SlowQueryLogger{wrapped: l.wrapped.LogMode(level), threshold: l.threshold, sink: l.sink}
}

func (l *SlowQueryLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	l.wrapped.Info(ctx, msg, data...)
}

func (l *SlowQueryLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.wrapped.Warn(ctx, msg, data...)
}

func (l *SlowQueryLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	l.wrapped.Error(ctx, msg, data...)
}

func (l *SlowQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.wrapped.Trace(ctx, begin, fc, err)

	elapsed := time.Since(begin)
	if l.threshold <= 0 || elapsed < l.threshold {
		return
	}
	sink := l.sink.Load()
	if sink == nil {
		return
	}
	sql, rows := fc()
	// Never report statements on the log table itself: the sink writes there,
	// and reporting those writes would feed back into the sink.
	if strings.Contains(sql, "log_entries") {
		return
	}
	(*sink)(SlowQueryEntry{SQL: sql, Elapsed: elapsed, Rows: rows, Source: slowQueryCaller()})
}

// slowQueryCaller returns the first stack frame outside GORM and this file,
// i.e. the application code that issued the query.
func slowQueryCaller() string {
	for i := 2; i < 18; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		if strings.Contains(file, "gorm.io") || strings.HasSuffix(file, "slow_query_logger.go") {
			continue
		}
		return fmt.Sprintf("%s:%d", file, line)
	}
	return ""
}
