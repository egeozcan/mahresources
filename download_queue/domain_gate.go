package download_queue

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DomainRule is one plugin-declared pacing rule as the queue needs it.
//
// The manifest package owns the author-facing grammar. application_context adapts
// that parsed manifest into these rules without making either leaf package import
// the other. Key must identify the manifest rule, not the submitted host, so a
// wildcard rule shares one concurrency/interval/backoff state across everything it
// matches. If Match is nil, Key is treated as an exact host match; this keeps tests
// and embedders that do not need wildcard grammar simple.
//
// This gate is deliberately job-level. One HLS job may make many segment requests
// under hls' own retry and concurrency rules; pacing individual segment requests
// is outside this queue-level seam. Generic jobs never enter processJob and have no
// remote URL to key by, so they are outside the gate as well.
type DomainRule struct {
	Key         string
	Concurrency int
	MinInterval time.Duration
	Backoff     time.Duration
	Match       func(host string) bool
}

// DomainPolicy is the per-plugin set of download pacing rules. First match wins,
// matching the manifest contract and letting authors put a specific exception
// before a broad wildcard.
type DomainPolicy struct {
	Rules []DomainRule
}

// ThrottleResolver answers which per-domain pacing rules a plugin has declared.
// ok=false means no limits are available for that plugin, and the gate is inert.
type ThrottleResolver func(pluginName string) (DomainPolicy, bool)

type matchedDomainRule struct {
	DomainRule
	key string
}

func (p DomainPolicy) matchURL(rawURL string) (matchedDomainRule, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return matchedDomainRule{}, false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return matchedDomainRule{}, false
	}
	for i, rule := range p.Rules {
		if !rule.matches(host) {
			continue
		}
		key := strings.TrimSpace(rule.Key)
		if key == "" {
			// A fallback for programmatic policies. Manifest adapters should provide
			// the source rule string as Key so wildcard rules share one state, but an
			// empty key must not collapse every empty-key rule into one bucket.
			key = "#" + strconv.Itoa(i)
		}
		return matchedDomainRule{DomainRule: rule, key: key}, true
	}
	return matchedDomainRule{}, false
}

func (r DomainRule) matches(host string) bool {
	if r.Match != nil {
		return r.Match(host)
	}
	return strings.EqualFold(r.Key, host)
}

type domainGateKey struct {
	pluginName string
	ruleKey    string
}

type domainGateState struct {
	running      int
	lastStart    time.Time
	backoffUntil time.Time
	changed      chan struct{}
}

// domainGate is process-local pacing state. It is intentionally not durable: a
// restart forgets interval and backoff memory, the same posture as other
// process-local coordination locks in this tree. It takes neither the manager lock
// nor any job lock while holding its own mutex, and never holds the mutex while it
// waits.
type domainGate struct {
	mu     sync.Mutex
	states map[domainGateKey]*domainGateState
}

type domainGateLease struct {
	gate        *domainGate
	key         domainGateKey
	rule        matchedDomainRule
	active      bool
	releaseSlot bool
}

func (g *domainGate) acquire(ctx context.Context, pluginName, rawURL string, policy DomainPolicy) (domainGateLease, error) {
	if pluginName == "" {
		return domainGateLease{}, nil
	}
	rule, ok := policy.matchURL(rawURL)
	if !ok {
		return domainGateLease{}, nil
	}
	key := domainGateKey{pluginName: pluginName, ruleKey: rule.key}
	lease := domainGateLease{gate: g, key: key, rule: rule, active: true}

	// Wait out existing backoff before taking the per-domain slot so a backed-off
	// host does not consume even its own declared concurrency while sleeping.
	if err := g.waitBackoff(ctx, key); err != nil {
		return domainGateLease{}, err
	}
	if rule.Concurrency > 0 {
		if err := g.acquireSlot(ctx, key, rule.Concurrency); err != nil {
			return domainGateLease{}, err
		}
		lease.releaseSlot = true
	}
	if err := g.waitUntilStart(ctx, key, rule); err != nil {
		lease.release()
		return domainGateLease{}, err
	}
	return lease, nil
}

func (g *domainGate) waitUntilStart(ctx context.Context, key domainGateKey, rule matchedDomainRule) error {
	for {
		if err := g.waitBackoff(ctx, key); err != nil {
			return err
		}
		var start time.Time
		if rule.MinInterval > 0 {
			start = g.reserveStart(key, rule.MinInterval)
		}
		if !start.IsZero() {
			if err := sleepUntil(ctx, start); err != nil {
				return err
			}
		}
		if !g.hasFutureBackoff(key, time.Now()) {
			return nil
		}
		// A 429/503 may have landed while this job was sleeping out its interval.
		// Loop and consume another conservative reservation after the backoff.
	}
}

func (g *domainGate) acquireSlot(ctx context.Context, key domainGateKey, limit int) error {
	for {
		g.mu.Lock()
		st := g.stateLocked(key)
		if st.running < limit {
			st.running++
			g.mu.Unlock()
			return nil
		}
		changed := st.changed
		g.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (g *domainGate) waitBackoff(ctx context.Context, key domainGateKey) error {
	for {
		g.mu.Lock()
		st := g.stateLocked(key)
		until := st.backoffUntil
		if until.IsZero() || !until.After(time.Now()) {
			g.mu.Unlock()
			return nil
		}
		changed := st.changed
		g.mu.Unlock()

		timer := time.NewTimer(time.Until(until))
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return ctx.Err()
		case <-changed:
			stopTimer(timer)
		case <-timer.C:
		}
	}
}

func (g *domainGate) reserveStart(key domainGateKey, interval time.Duration) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.stateLocked(key)
	now := time.Now()
	start := now
	if !st.lastStart.IsZero() {
		if next := st.lastStart.Add(interval); next.After(start) {
			start = next
		}
	}
	st.lastStart = start
	return start
}

func (g *domainGate) hasFutureBackoff(key domainGateKey, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.stateLocked(key)
	return !st.backoffUntil.IsZero() && st.backoffUntil.After(now)
}

func (g *domainGate) stateLocked(key domainGateKey) *domainGateState {
	if g.states == nil {
		g.states = make(map[domainGateKey]*domainGateState)
	}
	st := g.states[key]
	if st == nil {
		st = &domainGateState{changed: make(chan struct{})}
		g.states[key] = st
	}
	return st
}

func (g *domainGate) setBackoff(key domainGateKey, until time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.stateLocked(key)
	if until.After(st.backoffUntil) {
		st.backoffUntil = until
		g.wakeLocked(st)
	}
}

func (g *domainGate) wakeLocked(st *domainGateState) {
	close(st.changed)
	st.changed = make(chan struct{})
}

func (l domainGateLease) release() {
	if !l.active || !l.releaseSlot || l.gate == nil {
		return
	}
	l.gate.mu.Lock()
	defer l.gate.mu.Unlock()
	st := l.gate.stateLocked(l.key)
	if st.running > 0 {
		st.running--
		l.gate.wakeLocked(st)
	}
}

func (l domainGateLease) reportBackoff(err error) {
	if !l.active || l.gate == nil || l.rule.Backoff <= 0 || err == nil {
		return
	}
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) {
		return
	}
	if statusErr.Code != http.StatusTooManyRequests && statusErr.Code != http.StatusServiceUnavailable {
		return
	}
	delay := l.rule.Backoff
	if statusErr.RetryAfterOK && statusErr.RetryAfter < delay {
		delay = statusErr.RetryAfter
	}
	if delay <= 0 {
		return
	}
	l.gate.setBackoff(l.key, time.Now().Add(delay))
}

func sleepUntil(ctx context.Context, start time.Time) error {
	wait := time.Until(start)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer stopTimer(timer)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

// httpStatusError keeps the status code and Retry-After metadata at the only
// place they are visible. The job still prints the historical HTTP message, but
// processJob no longer has to parse err.Error() to apply domain backoff.
type httpStatusError struct {
	Code         int
	Status       string
	RetryAfter   time.Duration
	RetryAfterOK bool
}

func (e *httpStatusError) Error() string {
	return "HTTP " + strconv.Itoa(e.Code) + ": " + e.Status
}

func newHTTPStatusError(code int, status string, retryAfter string, now time.Time) *httpStatusError {
	err := &httpStatusError{Code: code, Status: status}
	if d, ok := parseRetryAfter(retryAfter, now); ok {
		err.RetryAfter = d
		err.RetryAfterOK = true
	}
	return err
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if retryAfterIsSeconds(value) {
		const maxDuration = time.Duration(1<<63 - 1)
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return maxDuration, true
		}
		if maxSeconds := int64(maxDuration / time.Second); seconds > maxSeconds {
			return maxDuration, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if !when.After(now) {
		return 0, true
	}
	return when.Sub(now), true
}

func retryAfterIsSeconds(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
