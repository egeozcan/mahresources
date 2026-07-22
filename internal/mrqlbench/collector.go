package mrqlbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm/logger"
)

type sampleContextKey struct{}

func WithSample(ctx context.Context, sampleID string) context.Context {
	return context.WithValue(ctx, sampleContextKey{}, sampleID)
}

type collectorState struct {
	mu      sync.Mutex
	samples map[string][]StatementObservation
}

type Collector struct {
	delegate logger.Interface
	state    *collectorState
}

func NewCollector(delegate logger.Interface) *Collector {
	if delegate == nil {
		delegate = logger.Discard
	}
	return &Collector{delegate: delegate, state: &collectorState{samples: map[string][]StatementObservation{}}}
}

func (c *Collector) LogMode(level logger.LogLevel) logger.Interface {
	return &Collector{delegate: c.delegate.LogMode(level), state: c.state}
}

func (c *Collector) Info(ctx context.Context, msg string, data ...interface{}) {
	c.delegate.Info(ctx, msg, data...)
}

func (c *Collector) Warn(ctx context.Context, msg string, data ...interface{}) {
	c.delegate.Warn(ctx, msg, data...)
}

func (c *Collector) Error(ctx context.Context, msg string, data ...interface{}) {
	c.delegate.Error(ctx, msg, data...)
}

func (c *Collector) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, rows := fc()
	c.delegate.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
	sampleID, _ := ctx.Value(sampleContextKey{}).(string)
	if sampleID == "" {
		return
	}
	observation := StatementObservation{
		Fingerprint:  sqlFingerprint(sql),
		Class:        sqlClass(sql),
		Rows:         rows,
		ElapsedNanos: time.Since(begin).Nanoseconds(),
	}
	if err != nil {
		observation.Error = fmt.Sprintf("%T", err)
	}
	c.state.mu.Lock()
	c.state.samples[sampleID] = append(c.state.samples[sampleID], observation)
	c.state.mu.Unlock()
}

func (c *Collector) Reset(sampleID string) {
	c.state.mu.Lock()
	delete(c.state.samples, sampleID)
	c.state.mu.Unlock()
}

func (c *Collector) Snapshot(sampleID string) []StatementObservation {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return append([]StatementObservation(nil), c.state.samples[sampleID]...)
}

var (
	sqlQuotedValue = regexp.MustCompile(`'(?:''|[^'])*'`)
	sqlNumberValue = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
	sqlWhitespace  = regexp.MustCompile(`\s+`)
)

func normalizedSQLShape(sql string) string {
	normalized := sqlQuotedValue.ReplaceAllString(sql, "?")
	normalized = sqlNumberValue.ReplaceAllString(normalized, "?")
	normalized = sqlWhitespace.ReplaceAllString(strings.TrimSpace(normalized), " ")
	return normalized
}

func sqlFingerprint(sql string) string {
	sum := sha256.Sum256([]byte(normalizedSQLShape(sql)))
	return "sql-shape-v1:" + hex.EncodeToString(sum[:])
}

func sqlClass(sql string) string {
	fields := strings.Fields(strings.TrimSpace(sql))
	if len(fields) == 0 {
		return "unknown"
	}
	first := strings.ToUpper(fields[0])
	if first == "WITH" {
		return "select"
	}
	switch first {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "PRAGMA", "EXPLAIN":
		return strings.ToLower(first)
	default:
		return "other"
	}
}
