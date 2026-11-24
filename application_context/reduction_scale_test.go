package application_context

// Scale proofs for the Reduction plan tables (design D14).
//
// These build libraries of up to two million Resources and run the public entry
// points over them, so they are off unless MAHRES_SCALE_TESTS=1. The same bodies
// run on SQLite (reduction_scale_test.go) and Postgres (reduction_scale_pg_test.go).
//
//	MAHRES_SCALE_TESTS=1 go test --tags 'json1 fts5' ./application_context -run 'TestScale' -v -timeout 4h
//	MAHRES_SCALE_TESTS=1 go test --tags 'json1 fts5 postgres' ./application_context -run 'TestScale.*PG' -v -timeout 4h
//
// MAHRES_SCALE_DIV divides every fixture size (default 1, the sizes the design
// names). A scenario that is expensive runs at two sizes, size/4 and size, and
// asserts the larger peak is within a fixed allowance of the smaller one: the
// bound is a constant, not a function of the library.
//
// What is asserted per scenario:
//   - peak Go heap above the post-GC baseline (runtime.MemStats.HeapInuse sampled
//     every 10 ms) under 64 MB for a request and 128 MB for compute and migration;
//   - no statement returns or writes more than one chunk (the GORM callback
//     below), except the named exemptions in scaleChunkExemptions;
//   - the Identical chunk query, the Identical holder query, the neighbour query
//     and the availability probe run on their indexes (EXPLAIN, both databases).
//
// Process RSS and wall time are logged, not asserted: they are properties of the
// machine.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

const (
	requestHeapLimitMB = 64
	computeHeapLimitMB = 128
	// scaleSizeAllowanceMB is how much the peak may grow between the quarter-size
	// and the full-size run before the bound counts as a function of the library.
	scaleSizeAllowanceMB = 16
)

func requireScale(t *testing.T) {
	t.Helper()
	if os.Getenv("MAHRES_SCALE_TESTS") == "" {
		t.Skip("scale proof: set MAHRES_SCALE_TESTS=1 (see the header of reduction_scale_test.go)")
	}
}

func scaleDiv() int {
	if v, err := strconv.Atoi(os.Getenv("MAHRES_SCALE_DIV")); err == nil && v > 0 {
		return v
	}
	return 1
}

// scaled is a design-named size divided by MAHRES_SCALE_DIV, at least min.
func scaled(full, min int) int {
	n := full / scaleDiv()
	if n < min {
		return min
	}
	return n
}

// ---- contexts ------------------------------------------------------------------------------

// newScaleSQLiteContext is a file-backed WAL database (the production shape: an
// in-memory shared-cache database would hold the whole fixture in the process).
func newScaleSQLiteContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=60000&_synchronous=NORMAL", filepath.Join(t.TempDir(), "scale.db"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Query{}, &models.Resource{}, &models.Note{}, &models.Tag{}, &models.Group{}, &models.Category{},
		&models.NoteType{}, &models.TemplateRevision{}, &models.Preview{}, &models.GroupRelation{}, &models.GroupRelationType{},
		&models.ImageHash{}, &models.ResourceSimilarity{}, &models.Job{}, &models.JobResourceReceipt{}, &models.LogEntry{},
		&models.ResourceCategory{}, &models.Series{}, &models.NoteBlock{}, &models.PluginKV{}, &models.ResourceVersion{},
		&models.User{}, &models.RuntimeSetting{},
	))
	require.NoError(t, db.AutoMigrate(reductionTables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{DbType: constants.DbTypeSqlite})
	rc := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	rc.ID = 1
	require.NoError(t, db.FirstOrCreate(rc, 1).Error)
	return withSettings(ctx)
}

func isPG(ctx *MahresourcesContext) bool { return ctx.Config.DbType == constants.DbTypePosgres }

// ---- measuring -----------------------------------------------------------------------------

type scaleResult struct {
	PeakMB float64
	BaseMB float64
	Wall   time.Duration
	RSSMB  int
}

func processRSSMB() int {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		return 0
	}
	kb, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return kb / 1024
}

// measure runs fn and reports the peak of HeapInuse above its value after a forced
// GC, sampled every 10 ms. The fixtures a test builds in Go are garbage by then, so
// what is left is the code under test.
func measure(t *testing.T, label string, fn func()) scaleResult {
	t.Helper()
	runtime.GC()
	debug.FreeOSMemory()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapInuse
	var peak atomic.Uint64
	peak.Store(base)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapInuse > peak.Load() {
					peak.Store(m.HeapInuse)
				}
			}
		}
	}()
	start := time.Now()
	fn()
	wall := time.Since(start)
	close(stop)
	<-done
	var end runtime.MemStats
	runtime.ReadMemStats(&end)
	if end.HeapInuse > peak.Load() {
		peak.Store(end.HeapInuse)
	}
	res := scaleResult{PeakMB: float64(peak.Load()-base) / (1 << 20), BaseMB: float64(base) / (1 << 20), Wall: wall, RSSMB: processRSSMB()}
	t.Logf("SCALE %-58s peak_heap=+%.1fMB (base %.0fMB) wall=%s rss=%dMB", label, res.PeakMB, res.BaseMB, wall.Round(time.Millisecond), res.RSSMB)
	return res
}

func requireUnder(t *testing.T, r scaleResult, limitMB float64, what string) {
	t.Helper()
	require.Less(t, r.PeakMB, limitMB, "%s: peak Go heap above baseline", what)
}

// ---- the chunk guard and the statement recorder -------------------------------------------

// scaleChunkExemptions are the statements allowed to return or write more than one
// chunk (500 rows), each with the reason. A statement matches when its normalized
// text (whitespace collapsed, literals and placeholders as ?) contains the fragment.
var scaleChunkExemptions = []struct{ Fragment, Reason string }{
	{"INSERT INTO reduction_migration_rows", "design D12: the one-time spool statement parses a legacy plan server-side in one INSERT ... SELECT per document, at startup, the single named exemption from rule 2"},
}

var (
	spaceRun       = regexp.MustCompile(`\s+`)
	placeholderRun = regexp.MustCompile(`\$\d+`)
	stringLiteral  = regexp.MustCompile(`'(?:[^']|'')*'`)
	numberLiteral  = regexp.MustCompile(`\b\d+\b`)
)

func normalizeSQL(sql string) string {
	return strings.TrimSpace(spaceRun.ReplaceAllString(placeholderRun.ReplaceAllString(sql, "?"), " "))
}

// shapeOf is a statement with its literals folded, so one shape is one key however
// many values it was sent with.
func shapeOf(sql string) string {
	return numberLiteral.ReplaceAllString(stringLiteral.ReplaceAllString(normalizeSQL(sql), "?"), "?")
}

type capturedStatement struct {
	SQL  string
	Vars []any
}

// scaleRecorder watches every statement on a context. Rows returned or written come
// from the logger's trace (the only hook that sees a Raw(...).Scan's row count); the
// SQL and its binds, for EXPLAIN, come from the query callbacks.
type scaleRecorder struct {
	mu        sync.Mutex
	armed     bool
	maxRows   map[string]int64
	patterns  map[string]*regexp.Regexp
	bindWant  map[string]string
	plans     map[string]string
	planErr   map[string]error
	captured  map[string]capturedStatement
	statCount int64
}

// scaleTracer is a gorm logger that records statement shapes and row counts and
// otherwise stays quiet (the default logger's slow-statement warnings drown a
// scale run).
type scaleTracer struct {
	logger.Interface
	rec *scaleRecorder
}

func (l scaleTracer) LogMode(level logger.LogLevel) logger.Interface {
	return scaleTracer{Interface: l.Interface.LogMode(level), rec: l.rec}
}

func (l scaleTracer) Trace(_ context.Context, _ time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	if err != nil || rows <= reductionRowChunk {
		l.rec.mu.Lock()
		l.rec.statCount++
		l.rec.mu.Unlock()
		return
	}
	shape := shapeOf(sql)
	if len(shape) > 160 {
		shape = shape[:160]
	}
	l.rec.mu.Lock()
	defer l.rec.mu.Unlock()
	l.rec.statCount++
	if l.rec.armed && rows > l.rec.maxRows[shape] {
		l.rec.maxRows[shape] = rows
	}
}

func installScaleRecorder(t *testing.T, ctx *MahresourcesContext) *scaleRecorder {
	t.Helper()
	rec := &scaleRecorder{maxRows: map[string]int64{}, patterns: map[string]*regexp.Regexp{}, bindWant: map[string]string{}, plans: map[string]string{}, planErr: map[string]error{}, captured: map[string]capturedStatement{}}
	ctx.db.Logger = scaleTracer{Interface: logger.Discard, rec: rec}
	watch := func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if sql == "" || strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "EXPLAIN") {
			return
		}
		norm := normalizeSQL(sql)
		vars := tx.Statement.Vars
		var hits []string
		rec.mu.Lock()
		for name, re := range rec.patterns {
			if _, ok := rec.captured[name]; ok || !re.MatchString(norm) {
				continue
			}
			if want := rec.bindWant[name]; want != "" {
				found := false
				for _, v := range vars {
					found = found || fmt.Sprint(v) == want
				}
				if !found {
					continue
				}
			}
			// Claimed now so a concurrent statement does not explain it twice.
			rec.captured[name] = capturedStatement{SQL: sql, Vars: append([]any(nil), vars...)}
			hits = append(hits, name)
		}
		rec.mu.Unlock()
		// EXPLAIN runs here, while the statement's tables hold what the compute
		// staged (the staging is cleared when the run lands, and a plan read
		// afterwards would be a plan over empty tables).
		for _, name := range hits {
			plan, err := explainStatement(ctx, sql, vars)
			rec.mu.Lock()
			rec.plans[name], rec.planErr[name] = plan, err
			rec.mu.Unlock()
		}
	}
	cb := ctx.db.Callback()
	require.NoError(t, cb.Query().After("gorm:query").Register("scale:query", watch))
	require.NoError(t, cb.Row().After("gorm:row").Register("scale:row", watch))
	require.NoError(t, cb.Raw().After("gorm:raw").Register("scale:raw", watch))
	return rec
}

func (r *scaleRecorder) arm(on bool) { r.mu.Lock(); r.armed = on; r.mu.Unlock() }

// capture keeps the first statement matching pattern; with a bind, the first one
// that also carries that value as a parameter (the hub's id, so the plan is the
// hub's and not a spoke's).
func (r *scaleRecorder) capture(name, pattern, bind string) {
	r.mu.Lock()
	r.patterns[name] = regexp.MustCompile(pattern)
	r.bindWant[name] = bind
	r.mu.Unlock()
}

// requireWithinChunk fails on any armed statement past a chunk that is not a named
// exemption, and logs the exemptions that fired.
func (r *scaleRecorder) requireWithinChunk(t *testing.T, scenario string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	var offenders []string
	keys := make([]string, 0, len(r.maxRows))
	for k := range r.maxRows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		exempt := ""
		for _, e := range scaleChunkExemptions {
			if strings.Contains(k, e.Fragment) {
				exempt = e.Reason
			}
		}
		if exempt != "" {
			t.Logf("SCALE %s: exempt statement (%d rows): %s -- %s", scenario, r.maxRows[k], k, exempt)
			continue
		}
		offenders = append(offenders, fmt.Sprintf("%d rows: %s", r.maxRows[k], k))
	}
	require.Empty(t, offenders, "%s: statements that returned or wrote more than %d rows", scenario, reductionRowChunk)
	r.maxRows = map[string]int64{}
	t.Logf("SCALE %s: no statement over %d rows outside the named exemptions (%d statements seen)", scenario, reductionRowChunk, r.statCount)
}

// explainStatement is the database's plan for a statement with its binds, one
// line per row.
func explainStatement(ctx *MahresourcesContext, sql string, vars []any) (string, error) {
	prefix := "EXPLAIN QUERY PLAN "
	if isPG(ctx) {
		prefix = "EXPLAIN "
	}
	rows, err := ctx.db.Raw(prefix+sql, vars...).Rows()
	if err != nil {
		return "", err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	var lines []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprint(vals[len(vals)-1]))
	}
	return strings.Join(lines, "\n"), rows.Err()
}

// explain returns the plan taken when the matching statement was first sent.
func (r *scaleRecorder) explain(t *testing.T, ctx *MahresourcesContext, name string) string {
	t.Helper()
	r.mu.Lock()
	plan, ok := r.plans[name]
	err := r.planErr[name]
	r.mu.Unlock()
	require.True(t, ok, "no statement matching %q was sent", name)
	require.NoError(t, err, "EXPLAIN of %s", name)
	t.Logf("SCALE EXPLAIN %s (%s):\n%s", name, map[bool]string{true: "postgres", false: "sqlite"}[isPG(ctx)], plan)
	return plan
}

// requireIndexed asserts the plan reads through an index and never scans a table
// whole. SQLite prints "SCAN <t>" for a full scan (a "SCAN CONSTANT ROW" is a
// literal row, not a table) and "SEARCH <t> USING ... INDEX <name>" for an index
// read; Postgres prints "Seq Scan on <t>". wantIndex is a fragment of the index the
// statement is built for, which must appear in the plan.
func requireIndexed(t *testing.T, ctx *MahresourcesContext, plan, name, wantIndex string) {
	t.Helper()
	require.Contains(t, plan, wantIndex, "%s uses the index it was built for", name)
	for _, line := range strings.Split(plan, "\n") {
		line = strings.TrimSpace(line)
		if isPG(ctx) {
			// Only the tables that are large in the fixture: Postgres rightly reads
			// a table of a few rows (this generation's Clusters) in one page.
			for _, big := range []string{"resources", "reduction_extent", "resource_similarities", "reduction_members", "image_hashes"} {
				require.NotRegexp(t, `Seq Scan on `+big+`\b`, line, "%s scans %s whole", name, big)
			}
			continue
		}
		if strings.HasPrefix(line, "SCAN ") && !strings.HasPrefix(line, "SCAN CONSTANT ROW") {
			require.Failf(t, "full scan", "%s: %s", name, line)
		}
	}
}

// ---- SQL fixtures --------------------------------------------------------------------------

// seedSeries is the recursive sequence both databases accept in front of an INSERT.
func seedSeries(n int) string {
	return fmt.Sprintf("WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < %d) ", n)
}

func maxID(t *testing.T, ctx *MahresourcesContext, table string) uint {
	t.Helper()
	var id uint
	require.NoError(t, ctx.db.Raw("SELECT COALESCE(MAX(id), 0) FROM "+table).Scan(&id).Error)
	return id
}

// seedResources inserts n Resources in one statement. The name, hash and owner are
// SQL expressions over n (1..n). It returns the first id; the ids are contiguous
// (asserted).
func seedResources(t *testing.T, ctx *MahresourcesContext, n int, nameExpr, hashExpr, ownerExpr string) uint {
	t.Helper()
	before := maxID(t, ctx, "resources")
	require.NoError(t, ctx.db.Exec(seedSeries(n)+`INSERT INTO resources (created_at, updated_at, name, hash, hash_type, content_type, file_size, width, height, resource_category_id, owner_id)
		SELECT CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, `+nameExpr+`, `+hashExpr+`, 'SHA1', 'image/png', 1000 + n % 977, 100, 100, 1, `+ownerExpr+` FROM seq`).Error)
	after := maxID(t, ctx, "resources")
	require.Equal(t, before+uint(n), after, "contiguous resource ids")
	return before + 1
}

func seedGroups(t *testing.T, ctx *MahresourcesContext, n int, ownerID uint) uint {
	t.Helper()
	before := maxID(t, ctx, "groups")
	owner := "NULL"
	if ownerID != 0 {
		owner = strconv.Itoa(int(ownerID))
	}
	require.NoError(t, ctx.db.Exec(seedSeries(n)+`INSERT INTO groups (created_at, updated_at, name, owner_id) SELECT CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'g' || n, `+owner+` FROM seq`).Error)
	require.Equal(t, before+uint(n), maxID(t, ctx, "groups"))
	return before + 1
}

func analyzeAll(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	require.NoError(t, ctx.db.Exec("ANALYZE").Error)
}

// buildSimilarityIndexes builds the two effective-distance indexes the neighbour
// query is written for (a server does it at startup) and logs how long they took
// over the pairs already stored.
func buildSimilarityIndexes(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	var pairs int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM resource_similarities").Scan(&pairs).Error)
	start := time.Now()
	require.NoError(t, models.EnsureSimilarityDistanceIndexes(ctx.db))
	t.Logf("SCALE similarity distance indexes built over %d pairs in %s", pairs, time.Since(start).Round(time.Millisecond))
	valid, err := models.SimilarityDistanceIndexesValid(ctx.db)
	require.NoError(t, err)
	require.True(t, valid)
}

// planSeed describes one Reduction's plan: clusters of 3 or 4 members (one in three
// has 4) over consecutive Resource ids from firstResource, Identical for two in
// three and Near for the rest.
type planSeed struct {
	Clusters      int
	FirstResource uint
	Checked       bool
	Generation    string
	Prefix        string
}

func (p planSeed) memberCount(i int) int {
	if i%3 == 0 {
		return 4
	}
	return 3
}

func (p planSeed) tier(i int) string {
	if i%3 == 2 {
		return models.ReductionTierNear
	}
	return models.ReductionTierIdentical
}

func (p planSeed) members() int {
	n := 0
	for i := 0; i < p.Clusters; i++ {
		n += p.memberCount(i)
	}
	return n
}

func (p planSeed) identical() int {
	n := 0
	for i := 0; i < p.Clusters; i++ {
		if p.tier(i) == models.ReductionTierIdentical {
			n++
		}
	}
	return n
}

// newScaleReduction creates a Reduction row marked as a whole-library, table-backed plan.
func newScaleReduction(t *testing.T, ctx *MahresourcesContext, name, generation string) *models.ResourceReduction {
	t.Helper()
	r := &models.ResourceReduction{Name: name, Status: models.ReductionStatusReady, MatchingMode: models.MatchingModeBothTiers}
	require.NoError(t, ctx.db.Create(r).Error)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 1, extent_all = ?, plan_generation = ? WHERE id = ?", true, generation, r.ID).Error)
	return loadReductionRow(t, ctx.db, r.ID)
}

// seedPlan writes the plan's rows through GORM in transactions of 5,000 Clusters,
// deriving every column the way the store does, so the derived-column invariant
// holds at scale.
func seedPlan(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, p planSeed) {
	t.Helper()
	resource := p.FirstResource
	const batch = 5000
	for from := 0; from < p.Clusters; from += batch {
		to := min(from+batch, p.Clusters)
		rows := make([]models.ReductionClusterRow, 0, to-from)
		memberSets := make([][]models.ReductionMemberRow, 0, to-from)
		for i := from; i < to; i++ {
			c := &storedCluster{Row: models.ReductionClusterRow{
				ReductionID: r.ID, Generation: p.Generation, Key: fmt.Sprintf("%s%07d", p.Prefix, i), Position: int64(i),
				Tier: p.tier(i), State: models.ReductionClusterOpen, Checked: p.Checked,
			}}
			hash := fmt.Sprintf("%sh%d", p.Prefix, i)
			for j := 0; j < p.memberCount(i); j++ {
				c.Members = append(c.Members, models.ReductionMemberRow{ResourceID: resource, Hash: hash, FileSize: int64(1000 + j), InExtent: true, Position: j})
				resource++
			}
			c.Row.WinnerID = c.Members[0].ResourceID
			deriveClusterColumns(c)
			rows = append(rows, c.Row)
			memberSets = append(memberSets, c.Members)
		}
		require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.CreateInBatches(&rows, 200).Error; err != nil {
				return err
			}
			members := make([]models.ReductionMemberRow, 0, (to-from)*4)
			for i := range rows {
				for _, m := range memberSets[i] {
					m.ClusterID, m.ReductionID, m.Generation = rows[i].ID, r.ID, p.Generation
					members = append(members, m)
				}
			}
			return tx.CreateInBatches(&members, 500).Error
		}))
	}
}

func rssLog(t *testing.T, label string) {
	t.Helper()
	t.Logf("SCALE %-58s rss=%dMB", label, processRSSMB())
}

// ---- scenario: review pages, list, bulk approve, scoped principal ---------------------------

// scaleReview builds a Reduction of `clusters` Clusters over `clusters*10/3`
// Resources owned by 20,000 Groups under one root, and reads it as an unscoped and
// as a root-scoped principal. It returns the largest peak of the page reads.
func scaleReview(t *testing.T, ctx *MahresourcesContext, clusters, groups int) (worstPageMB float64) {
	t.Helper()
	p := planSeed{Clusters: clusters, Generation: "gscale", Prefix: "k"}
	members := p.members()
	root := seedGroups(t, ctx, 1, 0)
	firstGroup := seedGroups(t, ctx, groups, root)
	start := time.Now()
	p.FirstResource = seedResources(t, ctx, members, `'r' || n`, `'x' || n`, fmt.Sprintf("%d + n %% %d", firstGroup, groups))
	r := newScaleReduction(t, ctx, "scale review", p.Generation)
	seedPlan(t, ctx, r, p)
	analyzeAll(t, ctx)
	t.Logf("SCALE review fixture: %d Clusters, %d members, %d Groups seeded in %s", clusters, members, groups, time.Since(start).Round(time.Second))

	// The recorder first: a principal's handle is a copy that keeps the logger it was made with.
	rec := installScaleRecorder(t, ctx)
	// Binding a scoped principal flattens its Group subtree into an allow-list for
	// every page in the app (docs/todo.md); the Reduction store does not use it. It
	// is measured here so the cost is on record, outside the store's assertions.
	var scoped, scopedLeaf *MahresourcesContext
	leaf := firstGroup + 17
	measure(t, fmt.Sprintf("WithPrincipal over a %d-Group subtree (app-wide, not the store)", groups), func() {
		scoped = ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &root})
	})
	scopedLeaf = ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 6, ScopeGroupID: &leaf})

	rec.arm(true)
	defer rec.arm(false)

	queries := []struct {
		name  string
		page  int
		query *query_models.ResourceReductionReviewQuery
	}{
		{"page 1 copies", 1, &query_models.ResourceReductionReviewQuery{PageSize: 100}},
		{"last page copies", (clusters + 99) / 100, &query_models.ResourceReductionReviewQuery{PageSize: 100}},
		{"page 1 size sort", 1, &query_models.ResourceReductionReviewQuery{PageSize: 100, Sort: "size"}},
		{"page 1 computed, near, attention", 1, &query_models.ResourceReductionReviewQuery{PageSize: 100, Sort: "computed", Tier: []string{models.ReductionTierNear}, NeedsAttention: true}},
		{"page 1 approved filter", 1, &query_models.ResourceReductionReviewQuery{PageSize: 100, Approval: "approved"}},
	}
	read := func(label string, c *MahresourcesContext) {
		for _, q := range queries {
			res := measure(t, label+" review "+q.name, func() {
				review, err := c.GetReductionReview(r.ID, nil, false, q.page, q.query)
				require.NoError(t, err)
				// The .json twin encodes the same view.
				_, err = json.Marshal(review)
				require.NoError(t, err)
			})
			requireUnder(t, res, requestHeapLimitMB, label+" review "+q.name)
			worstPageMB = max(worstPageMB, res.PeakMB)
		}
	}
	read("unscoped", ctx)
	read("scoped(20k Groups)", scoped)
	read("scoped(one Group)", scopedLeaf)
	rec.requireWithinChunk(t, "review")
	return worstPageMB
}

// scaleList seeds extra Reductions beside the first so the list shows `n` large ones.
func scaleList(t *testing.T, ctx *MahresourcesContext, n, clustersEach int) {
	t.Helper()
	p := planSeed{Clusters: clustersEach, Generation: "glist", Prefix: "l"}
	first := seedResources(t, ctx, p.members(), `'lr' || n`, `'lx' || n`, `NULL`)
	for i := 0; i < n; i++ {
		r := newScaleReduction(t, ctx, fmt.Sprintf("scale list %d", i), p.Generation)
		q := p
		q.FirstResource = first
		q.Prefix = fmt.Sprintf("l%d-", i)
		seedPlan(t, ctx, r, q)
	}
	analyzeAll(t, ctx)
	rec := installScaleRecorder(t, ctx)
	rec.arm(true)
	defer rec.arm(false)
	res := measure(t, fmt.Sprintf("list of %d Reductions", n), func() {
		rows, total, progress, err := ctx.ListResourceReductions(0, 50, &query_models.ResourceReductionQuery{})
		require.NoError(t, err)
		require.GreaterOrEqual(t, total, int64(n))
		require.NotEmpty(t, rows)
		require.NotEmpty(t, progress)
		_, err = json.Marshal(progress)
		require.NoError(t, err)
	})
	requireUnder(t, res, requestHeapLimitMB, "list")
	rec.requireWithinChunk(t, "list")
}

// scaleBulkApprove checks every open Identical Cluster of a Reduction in one request.
func scaleBulkApprove(t *testing.T, ctx *MahresourcesContext, clusters int) {
	t.Helper()
	p := planSeed{Clusters: clusters, Generation: "gbulk", Prefix: "b"}
	p.FirstResource = seedResources(t, ctx, p.members(), `'br' || n`, `'bx' || n`, `NULL`)
	r := newScaleReduction(t, ctx, "scale bulk", p.Generation)
	seedPlan(t, ctx, r, p)
	analyzeAll(t, ctx)
	rec := installScaleRecorder(t, ctx)
	rec.arm(true)
	defer rec.arm(false)
	expected := uint(p.identical())
	var result any
	res := measure(t, fmt.Sprintf("bulk approve allIdentical (%d Clusters)", p.identical()), func() {
		out, err := ctx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
			ID: r.ID, Version: r.Version, Action: ReductionActionCheck, AllIdentical: true, ExpectedCount: &expected,
		}, nil, false)
		require.NoError(t, err)
		result = out
	})
	t.Logf("SCALE bulk approve result: %+v; the one transaction above is the whole writer-lock hold on SQLite", result)
	requireUnder(t, res, requestHeapLimitMB, "bulk approve allIdentical")
	rec.requireWithinChunk(t, "bulk approve")
	var checked int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ? AND checked", r.ID).Scan(&checked).Error)
	require.EqualValues(t, p.identical(), checked)
	assertBulkMarksCleared(t, ctx, r.ID)

	// The apply's stamp over the Clusters just approved: one transaction of
	// id-ordered chunks, so its duration is the writer-lock hold of an apply claim.
	batch := newClaimID()
	rec.arm(true)
	stamp := measure(t, fmt.Sprintf("apply stamp over %d approved Clusters (one transaction)", p.identical()), func() {
		require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
			return ctx.reductionStore().in(tx).stampBatch(r, p.Generation, batch)
		}))
	})
	requireUnder(t, stamp, requestHeapLimitMB, "apply stamp")
	var token string
	var stamped, losers int
	measure(t, "apply token over the stamped batch", func() {
		var err error
		token, stamped, losers, err = ctx.reductionStore().batchToken(r.ID, p.Generation, batch)
		require.NoError(t, err)
	})
	require.Equal(t, p.identical(), stamped)
	require.NotEmpty(t, token)
	t.Logf("SCALE apply stamp: %d Clusters, %d Losers; the stamp's wall time above is the whole writer-lock hold on SQLite", stamped, losers)
	rec.requireWithinChunk(t, "apply stamp")
}

func assertBulkMarksCleared(t *testing.T, ctx *MahresourcesContext, reductionID uint) {
	t.Helper()
	var n int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ? AND bulk_op <> ''", reductionID).Scan(&n).Error)
	require.Zero(t, n)
}

// scaleApply applies `clusters` approved two-member Identical Clusters over real Resources.
func scaleApply(t *testing.T, ctx *MahresourcesContext, clusters int) {
	t.Helper()
	first := seedResources(t, ctx, clusters*2, `'ar' || n`, `'ah' || ((n + 1) / 2)`, `NULL`)
	r := newScaleReduction(t, ctx, "scale apply", "gapply")
	rows := make([]models.ReductionClusterRow, 0, clusters)
	sets := make([][]models.ReductionMemberRow, 0, clusters)
	for i := 0; i < clusters; i++ {
		w, l := first+uint(2*i), first+uint(2*i+1)
		hash := fmt.Sprintf("ah%d", i+1)
		c := &storedCluster{Row: models.ReductionClusterRow{ReductionID: r.ID, Generation: "gapply", Key: fmt.Sprintf("a%06d", i), Position: int64(i),
			Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen, Checked: true, WinnerID: w},
			Members: []models.ReductionMemberRow{{ResourceID: w, Hash: hash, FileSize: 1000, InExtent: true}, {ResourceID: l, Hash: hash, FileSize: 1000, InExtent: true, Position: 1}}}
		deriveClusterColumns(c)
		rows = append(rows, c.Row)
		sets = append(sets, c.Members)
	}
	require.NoError(t, ctx.db.CreateInBatches(&rows, 200).Error)
	var members []models.ReductionMemberRow
	for i := range rows {
		for _, m := range sets[i] {
			m.ClusterID, m.ReductionID, m.Generation = rows[i].ID, r.ID, "gapply"
			members = append(members, m)
		}
	}
	require.NoError(t, ctx.db.CreateInBatches(&members, 500).Error)
	analyzeAll(t, ctx)
	rec := installScaleRecorder(t, ctx)
	rec.arm(true)
	defer rec.arm(false)
	var result any
	res := measure(t, fmt.Sprintf("apply of %d Clusters", clusters), func() {
		out, err := ctx.ApplyResourceReduction(&query_models.ReductionApply{ID: r.ID, Version: r.Version}, nil, false)
		require.NoError(t, err)
		require.Equal(t, clusters, out.AppliedCount+out.StaleCount+out.Changed+out.Errors, "every Cluster has an outcome: %+v", out)
		require.Zero(t, out.Errors)
		result = fmt.Sprintf("applied=%d stale=%d changed=%d destroyed=%d more=%v", out.AppliedCount, out.StaleCount, out.Changed, out.Destroyed, out.More)
	})
	t.Logf("SCALE apply result: %v", result)
	requireUnder(t, res, requestHeapLimitMB, "apply")
	rec.requireWithinChunk(t, "apply")
	var left int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM resources WHERE name LIKE 'ar%'").Scan(&left).Error)
	require.EqualValues(t, clusters, left, "each Cluster merged its Loser away")
}

// ---- scenario: bulk delete with a Resource of many pairs; the 200-Loser merge ---------------

func scaleBulkDelete(t *testing.T, ctx *MahresourcesContext, deleteIDs, pairs int) {
	t.Helper()
	total := max(deleteIDs, pairs+1) + 1
	first := seedResources(t, ctx, total, `'dr' || n`, `'dx' || n`, `NULL`)
	center := first
	// center pairs with the next `pairs` Resources (smaller id first, as the writer stores them).
	require.NoError(t, ctx.db.Exec(seedSeries(pairs)+`INSERT INTO resource_similarities (resource_id1, resource_id2, hamming_distance, p_distance, a_distance)
		SELECT ?, ? + n, 2, 2, 2 FROM seq`, center, center).Error)
	var npairs int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM resource_similarities WHERE resource_id1 = ?", center).Scan(&npairs).Error)
	require.EqualValues(t, pairs, npairs)
	buildSimilarityIndexes(t, ctx)
	analyzeAll(t, ctx)
	ids := make([]uint, deleteIDs)
	for i := range ids {
		ids[i] = first + uint(i)
	}
	rec := installScaleRecorder(t, ctx)
	rec.arm(true)
	defer rec.arm(false)
	res := measure(t, fmt.Sprintf("bulk delete of %d ids (one with %d pairs)", deleteIDs, pairs), func() {
		require.NoError(t, ctx.BulkDeleteResources(&query_models.BulkQuery{ID: ids}))
	})
	requireUnder(t, res, requestHeapLimitMB, "bulk delete")
	rec.requireWithinChunk(t, "bulk delete")
	var left int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM resources WHERE id >= ? AND id < ?", first, first+uint(deleteIDs)).Scan(&left).Error)
	require.Zero(t, left)
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM resource_similarities WHERE resource_id1 = ?", center).Scan(&npairs).Error)
	require.Zero(t, npairs, "the pairs of the deleted Resource went with it")
}

func scaleMerge(t *testing.T, ctx *MahresourcesContext, losers, backupBytes int) {
	t.Helper()
	first := seedResources(t, ctx, losers+1, `'mr' || n`, `'same'`, `NULL`)
	winner := first
	// The Winner's history: "backups" holds one snapshot per earlier merge. Built in
	// pieces so the fixture's own peak stays modest, then dropped before measuring.
	piece := strings.Repeat("x", 100_000)
	var b strings.Builder
	b.WriteString(`{"backups":{`)
	for i := 0; b.Len() < backupBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"2020-01-01T00:00:%02d.%06dZ":{"id":%d,"name":"old%d","note":"%s"}`, i%60, i, i+1, i, piece)
	}
	b.WriteString(`}}`)
	require.NoError(t, ctx.db.Exec("UPDATE resources SET meta = ? WHERE id = ?", b.String(), winner).Error)
	b.Reset()
	piece = ""
	analyzeAll(t, ctx)
	ids := make([]uint, losers)
	for i := range ids {
		ids[i] = first + 1 + uint(i)
	}
	rec := installScaleRecorder(t, ctx)
	rec.arm(true)
	defer rec.arm(false)
	res := measure(t, fmt.Sprintf("merge of %d Losers into a Winner with %d MB of backups", losers, backupBytes>>20), func() {
		require.NoError(t, ctx.MergeResources(winner, ids, false))
	})
	requireUnder(t, res, requestHeapLimitMB, "merge")
	rec.requireWithinChunk(t, "merge")
	var left int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM resources WHERE id > ? AND id <= ?", winner, winner+uint(losers)).Scan(&left).Error)
	require.Zero(t, left)
}

// ---- scenario: compute -------------------------------------------------------------------

// computeFixture is the hash-only library of the design: `unique` Resources with
// distinct hashes, five more sharing one hash that sorts last, a hash held by
// `holders` Resources, and a dense star of `spokes` image-hashed Resources around
// one centre.
type computeFixture struct {
	Unique, Holders, Spokes int
}

func seedComputeLibrary(t *testing.T, ctx *MahresourcesContext, f computeFixture) (hub uint) {
	t.Helper()
	seedResources(t, ctx, f.Unique, `'u' || n`, `'h' || n`, `NULL`)
	seedResources(t, ctx, 5, `'dup' || n`, `'zz-last'`, `NULL`)
	seedResources(t, ctx, f.Holders, `'holder' || n`, `'zz-holders'`, `NULL`)
	center := seedResources(t, ctx, f.Spokes+1, `'star' || n`, `'sh' || n`, `NULL`)
	// The hub is the largest file, so it is the first seed the default Winner Rule
	// reads and its whole star is built from it.
	require.NoError(t, ctx.db.Exec(`UPDATE resources SET file_size = 1000000000 WHERE id = ?`, center).Error)
	require.NoError(t, ctx.db.Exec(`INSERT INTO image_hashes (resource_id, p_hash_int, status) SELECT id, id, 'ok' FROM resources WHERE id >= ? AND id <= ?`, center, center+uint(f.Spokes)).Error)
	require.NoError(t, ctx.db.Exec(seedSeries(f.Spokes)+`INSERT INTO resource_similarities (resource_id1, resource_id2, hamming_distance, p_distance, a_distance)
		SELECT ?, ? + n, 1 + n % 4, 1 + n % 4, 1 + n % 4 FROM seq`, center, center).Error)
	// Other Reductions' plans: the anti-join the compute asks is over a members table
	// that is large in production, so the planner must choose its index on real
	// statistics, not on an empty table.
	other := f.Unique / 2
	require.NoError(t, ctx.db.Exec(seedSeries(other)+`INSERT INTO reduction_members (cluster_id, reduction_id, generation, resource_id, position, hash, file_size)
		SELECT n, 999999, 'gother', n, 0, 'o' || n, 1000 FROM seq`).Error)
	buildSimilarityIndexes(t, ctx)
	analyzeAll(t, ctx)
	return center
}

func scaleCompute(t *testing.T, ctx *MahresourcesContext, f computeFixture) float64 {
	t.Helper()
	start := time.Now()
	hub := seedComputeLibrary(t, ctx, f)
	t.Logf("SCALE compute fixture: %d unique + 5 duplicates + %d holders + a %d-spoke star seeded in %s", f.Unique, f.Holders, f.Spokes, time.Since(start).Round(time.Second))
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	rec := installScaleRecorder(t, ctx)
	rec.capture("identical chunk", `SELECT hash FROM reduction_extent WHERE reduction_id = \? AND generation = \? AND hash > \?`, "")
	rec.capture("identical holders", `FROM resources r WHERE r.hash = \? AND r.id > \?`, "zz-holders")
	rec.capture("neighbours", `FROM resource_similarities s WHERE s\.resource_id1 = \?`, fmt.Sprint(hub))
	rec.capture("availability probe", `SELECT COUNT\(\*\) FROM resources r WHERE r.id = \?.*NOT EXISTS \(SELECT 1 FROM reduction_members`, "")
	rec.arm(true)
	rssLog(t, "compute: before")
	res := measure(t, fmt.Sprintf("compute over %d Resources", f.Unique+5+f.Holders+f.Spokes+1), func() {
		streamCompute(t, ctx, r)
	})
	rec.arm(false)
	requireUnder(t, res, computeHeapLimitMB, "compute")
	rec.requireWithinChunk(t, "compute")

	plan := planOf(t, ctx, r)
	var dup, holders, star *planRow
	for _, p := range plan {
		p := p
		switch {
		case len(p.Members) == 5 && p.Row.Tier == models.ReductionTierIdentical:
			dup = &p
		case p.Row.Tier == models.ReductionTierIdentical && p.Row.MemberCount == models.MaxClusterMembers:
			holders = &p
		case p.Row.Tier == models.ReductionTierNear:
			star = &p
		}
	}
	require.NotNil(t, dup, "the five duplicates at the end of the hash order form a Cluster")
	require.NotNil(t, holders, "the 50k-holder hash forms a capped Cluster")
	require.Equal(t, f.Holders-models.MaxClusterMembers, holders.Row.Truncated, "holders past the member cap are counted, not loaded")
	require.NotNil(t, star, "the dense star forms a Cluster")
	require.Equal(t, models.MaxClusterMembers, star.Row.MemberCount, "a star larger than the cap is cut at the cap")
	require.True(t, star.Row.FixedSeed, "199 insiders found: a full star keeps its seed")
	require.Equal(t, hub, star.Row.WinnerID)
	require.Equal(t, len(plan), 3+0, "no other Cluster: every other hash is unique")

	// The index each statement is built for, by database.
	want := map[string][2]string{
		"identical chunk":    {"idx_re_hash", "idx_re_hash"},
		"identical holders":  {"idx_resources_hash", "idx_resources_hash"},
		"neighbours":         {"idx_sim_r1_eff_dist", "idx_sim_r1_eff_dist"},
		"availability probe": {"idx_rm_resource", "idx_rm_resource"},
	}
	for name, idx := range want {
		wantIndex := idx[0]
		if isPG(ctx) {
			wantIndex = idx[1]
		}
		requireIndexed(t, ctx, rec.explain(t, ctx, name), name, wantIndex)
	}
	return res.PeakMB
}

// ---- scenario: migration -----------------------------------------------------------------

func scaleMigrate(t *testing.T, ctx *MahresourcesContext, bigMembers, smallClusters int) float64 {
	t.Helper()
	resources := bigMembers + smallClusters*3
	first := seedResources(t, ctx, resources, `'mg' || n`, `'mh' || n`, `NULL`)
	type m = map[string]any
	var members []any
	for i := 0; i < bigMembers; i++ {
		members = append(members, m{"resourceId": first + uint(i), "hash": "big", "inExtent": true})
	}
	clusters := []any{m{"id": "c-big", "tier": "identical", "winnerId": first, "state": "open", "members": members}}
	members = nil
	next := first + uint(bigMembers)
	for i := 0; i < smallClusters; i++ {
		clusters = append(clusters, m{"id": fmt.Sprintf("c-%06d", i), "tier": "identical", "winnerId": next, "state": "open", "checked": true,
			"members": []any{
				m{"resourceId": next, "hash": "s", "inExtent": true}, m{"resourceId": next + 1, "hash": "s", "inExtent": true}, m{"resourceId": next + 2, "hash": "s", "inExtent": true},
			}})
		next += 3
	}
	plan, err := json.Marshal(m{"clusters": clusters, "coverage": m{"extentSize": resources, "contentHashed": resources}, "settings": m{"matchingMode": "both", "winnerRule": []string{"pixels"}}})
	require.NoError(t, err)
	clusters = nil
	extent, _ := json.Marshal(m{"all": true})
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy scale", Status: models.ReductionStatusReady, MatchingMode: "both"}, Plan: plan, Extent: extent}
	require.NoError(t, ctx.db.Create(legacy).Error)
	t.Logf("SCALE migration fixture: legacy plan of %d MB, one %d-member Cluster and %d small ones", len(plan)>>20, bigMembers, smallClusters)
	plan = nil
	analyzeAll(t, ctx)

	rec := installScaleRecorder(t, ctx)
	rec.arm(true)
	rssLog(t, "migration: before")
	res := measure(t, "migration of a legacy plan", func() {
		require.NoError(t, ctx.MigrateReductionPlans())
	})
	rec.arm(false)
	requireUnder(t, res, computeHeapLimitMB, "migration")
	rec.requireWithinChunk(t, "migration")
	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, legacy.ID).Error)
	require.Equal(t, 1, got.PlanSchema)
	var n int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ?", legacy.ID).Scan(&n).Error)
	require.EqualValues(t, smallClusters+1, n)
	var big models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND key = ?", legacy.ID, "c-big").First(&big).Error)
	require.True(t, big.ReadOnly, "an over-cap legacy Cluster is read-only history")
	require.Equal(t, bigMembers, big.MemberCount, "an over-cap Cluster's counts come from SQL aggregates, not from hydrating it")
	require.Zero(t, big.GoneMembers)
	require.Empty(t, big.Fingerprint, "a read-only Cluster is never actionable and has no fingerprint")
	var unfingerprinted int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ? AND key <> 'c-big' AND fingerprint = ''", legacy.ID).Scan(&unfingerprinted).Error)
	require.Zero(t, unfingerprinted)
	return res.PeakMB
}

// ---- the wrappers shared by both databases -------------------------------------------------

// twoSizes runs body at a quarter of the size and at the size, each on a fresh
// context, and requires the peak not to grow with the library.
func twoSizes(t *testing.T, newCtx func(*testing.T) *MahresourcesContext, name string, body func(t *testing.T, ctx *MahresourcesContext, divisor int) float64) {
	t.Helper()
	var small, large float64
	t.Run("quarter-size", func(t *testing.T) { small = body(t, newCtx(t), 4) })
	t.Run("full-size", func(t *testing.T) { large = body(t, newCtx(t), 1) })
	t.Logf("SCALE %s: peak at a quarter of the size +%.1fMB, at the size +%.1fMB", name, small, large)
	require.LessOrEqual(t, large, small+scaleSizeAllowanceMB, "%s: the peak grows with the library", name)
}

func runScaleReview(t *testing.T, newCtx func(*testing.T) *MahresourcesContext) {
	requireScale(t)
	twoSizes(t, newCtx, "review", func(t *testing.T, ctx *MahresourcesContext, d int) float64 {
		return scaleReview(t, ctx, scaled(300_000, 2_000)/d, scaled(20_000, 100)/d)
	})
}

func runScaleListAndBulk(t *testing.T, newCtx func(*testing.T) *MahresourcesContext) {
	requireScale(t)
	t.Run("list", func(t *testing.T) { scaleList(t, newCtx(t), 50, scaled(6_000, 100)) })
	t.Run("bulk-approve", func(t *testing.T) { scaleBulkApprove(t, newCtx(t), scaled(300_000, 2_000)) })
}

func runScaleApply(t *testing.T, newCtx func(*testing.T) *MahresourcesContext) {
	requireScale(t)
	scaleApply(t, newCtx(t), scaled(2_000, 50))
}

func runScaleDeleteAndMerge(t *testing.T, newCtx func(*testing.T) *MahresourcesContext) {
	requireScale(t)
	t.Run("bulk-delete", func(t *testing.T) { scaleBulkDelete(t, newCtx(t), scaled(10_000, 500), scaled(1_000_000, 5_000)) })
	t.Run("merge", func(t *testing.T) { scaleMerge(t, newCtx(t), models.MaxClusterMembers-1, 50<<20/scaleDivOrOne()) })
}

func scaleDivOrOne() int { return min(scaleDiv(), 10) }

func runScaleCompute(t *testing.T, newCtx func(*testing.T) *MahresourcesContext) {
	requireScale(t)
	twoSizes(t, newCtx, "compute", func(t *testing.T, ctx *MahresourcesContext, d int) float64 {
		return scaleCompute(t, ctx, computeFixture{
			Unique: scaled(2_000_000, 20_000) / d, Holders: scaled(50_000, 1_000) / d, Spokes: max(scaled(20_000, 5_000)/d, 5_000),
		})
	})
}

func runScaleMigration(t *testing.T, newCtx func(*testing.T) *MahresourcesContext) {
	requireScale(t)
	twoSizes(t, newCtx, "migration", func(t *testing.T, ctx *MahresourcesContext, d int) float64 {
		return scaleMigrate(t, ctx, scaled(50_000, 1_000)/d, scaled(50_000, 500)/d)
	})
}

func TestScaleReviewSQLite(t *testing.T)         { runScaleReview(t, newScaleSQLiteContext) }
func TestScaleListAndBulkSQLite(t *testing.T)    { runScaleListAndBulk(t, newScaleSQLiteContext) }
func TestScaleApplySQLite(t *testing.T)          { runScaleApply(t, newScaleSQLiteContext) }
func TestScaleDeleteAndMergeSQLite(t *testing.T) { runScaleDeleteAndMerge(t, newScaleSQLiteContext) }
func TestScaleComputeSQLite(t *testing.T)        { runScaleCompute(t, newScaleSQLiteContext) }
func TestScaleMigrationSQLite(t *testing.T)      { runScaleMigration(t, newScaleSQLiteContext) }

var _ = context.Background
