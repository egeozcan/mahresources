package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"mahresources/contracts"
	"mahresources/mrql"
)

type MetadataIndexer struct {
	db     *gorm.DB
	runMu  sync.Mutex
	mu     sync.RWMutex
	status contracts.MetadataIndexStatus
	ready  []mrql.MetadataIndex
}

func NewMetadataIndexer(db *gorm.DB) *MetadataIndexer {
	return &MetadataIndexer{db: db, status: contracts.MetadataIndexStatus{State: "pending"}}
}

func (m *MetadataIndexer) Status() contracts.MetadataIndexStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// ReadyIndexes returns a snapshot of declarations whose entire physical index
// set completed reconciliation. Query translation must not pay index-specific
// predicate costs for keys that have no usable index.
func (m *MetadataIndexer) ReadyIndexes() []mrql.MetadataIndex {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]mrql.MetadataIndex(nil), m.ready...)
}

func (m *MetadataIndexer) report(state, detail string, err error) {
	status := contracts.MetadataIndexStatus{State: state, Detail: detail, CheckedAt: time.Now()}
	if err != nil {
		status.Error = err.Error()
	}
	m.mu.Lock()
	status.Configuration = m.status.Configuration
	m.status = status
	m.mu.Unlock()
}

func (m *MetadataIndexer) Run(ctx context.Context) {
	lastError := ""
	for ctx.Err() == nil {
		// Index builds have their own budget; they must not inherit MRQL's short
		// interactive deadline.
		workCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		err := m.Reconcile(workCtx)
		cancel()
		delay := 5 * time.Second
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if err.Error() != lastError {
				log.Printf("metadata indexes: %v", err)
			}
			lastError = err.Error()
			delay = time.Minute
		} else {
			lastError = ""
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// Reconcile reads the durable desired state every time, so restarts, category changes,
// failed concurrent builds, and multiple server processes converge. Invalid
// configuration never causes an index to be dropped.
func (m *MetadataIndexer) Reconcile(ctx context.Context) error {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	var verified []mrql.MetadataIndex
	reconcile := func(db *gorm.DB) error {
		var err error
		verified, err = m.reconcileOn(db)
		return err
	}
	err := m.db.WithContext(ctx).Connection(func(db *gorm.DB) error {
		// Connection supplies a mutable GORM statement; clone each operation
		// so prior catalog SQL cannot contaminate later operations.
		db = db.Session(&gorm.Session{})
		switch db.Dialector.Name() {
		case "postgres":
			// Session lock on the reserved connection, outside a transaction:
			// CREATE/DROP INDEX CONCURRENTLY are forbidden inside transactions.
			var locked bool
			if err := db.Raw("SELECT pg_try_advisory_lock(734639214822)").Scan(&locked).Error; err != nil {
				return err
			}
			if !locked {
				m.report("pending", "Another server is updating metadata indexes", nil)
				return nil
			}
			defer func() {
				unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				db.WithContext(unlockCtx).Exec("SELECT pg_advisory_unlock(734639214822)")
			}()
			return reconcile(db)
		case "sqlite":
			// SQLite has one writer. Atomic DDL also prevents a partial reconciliation
			// if a later index fails (for example because stored JSON is malformed).
			return db.Transaction(reconcile)
		default:
			return fmt.Errorf("metadata indexes are unsupported on %q", db.Dialector.Name())
		}
	})
	// Publish only after the connection callback (and SQLite transaction) succeeds.
	// A failure or another server's in-progress build falls back to ordinary SQL.
	m.mu.Lock()
	m.ready = nil
	if err == nil {
		m.ready = verified
	}
	m.mu.Unlock()
	if err != nil {
		m.report("failed", m.Status().Detail, err)
	} else if m.Status().State != "pending" {
		m.report("ready", "All configured metadata indexes are ready", nil)
	}
	return err
}

type managedMetadataIndex struct {
	Name       string
	SchemaName string
	TableName  string
	Valid      bool
}

// Only this reserved namespace is managed. Ordinary/operator-created indexes
// remain untouched. The version allows replacement if expressions change.
var managedMetadataIndexName = regexp.MustCompile(`^mah_midx_v[0-9]+_[0-9a-f]{32}$`)

func (m *MetadataIndexer) reconcileOn(db *gorm.DB) ([]mrql.MetadataIndex, error) {
	desired, err := collectCategoryMetadataIndexes(db)
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(desired)
	raw := string(encoded)
	m.mu.Lock()
	m.status.Configuration = raw
	m.mu.Unlock()
	m.report("checking", "Checking configured metadata indexes", nil)
	existing, err := listManagedMetadataIndexes(db)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	// Build replacements before removing old indexes. A failed new build must
	// not discard a previously useful index.
	for _, index := range desired {
		physical, err := index.Indexes(db.Dialector.Name())
		if err != nil {
			return nil, err
		}
		built := false
		for _, part := range physical {
			wanted[part.Name] = true
			current, ok := existing[part.Name]
			if ok && current.TableName != index.Table() {
				return nil, fmt.Errorf("managed index %s is on an unexpected table", part.Name)
			}
			if ok && current.Valid {
				continue
			}
			detail := fmt.Sprintf("%s.%s (%s)", index.Entity, index.Key, index.Kind)
			m.report("building", detail, nil)
			if ok {
				// Interrupted concurrent builds leave invalid catalog entries.
				if err := dropManagedMetadataIndex(db, current); err != nil {
					return nil, err
				}
			}
			if err := db.Exec(part.SQL).Error; err != nil {
				return nil, fmt.Errorf("build %s: %w", detail, err)
			}
			built = true
		}
		// Expression statistics are needed before the planner can cost the indexes.
		if built {
			if err := db.Exec("ANALYZE " + quoteMetadataIdentifier(index.Table())).Error; err != nil {
				return nil, fmt.Errorf("analyze %s: %w", index.Table(), err)
			}
		}
	}
	for name, index := range existing {
		if wanted[name] {
			continue
		}
		m.report("removing", "Removing an index no longer configured", nil)
		if err := dropManagedMetadataIndex(db, index); err != nil {
			return nil, err
		}
	}
	return desired, nil
}

func listManagedMetadataIndexes(db *gorm.DB) (map[string]managedMetadataIndex, error) {
	var rows []managedMetadataIndex
	var err error
	if db.Dialector.Name() == "postgres" {
		err = db.Raw(`SELECT c.relname AS name, ns.nspname AS schema_name, t.relname AS table_name, i.indisvalid AS valid
   FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
   JOIN pg_namespace ns ON ns.oid = c.relnamespace JOIN pg_class t ON t.oid = i.indrelid
   WHERE i.indrelid IN (to_regclass('resources'), to_regclass('notes'), to_regclass('groups'))`).Scan(&rows).Error
	} else {
		err = db.Raw(`SELECT name, tbl_name AS table_name, 1 AS valid FROM sqlite_master
   WHERE type = 'index' AND tbl_name IN ('resources', 'notes', 'groups')`).Scan(&rows).Error
	}
	if err != nil {
		return nil, err
	}
	indexes := map[string]managedMetadataIndex{}
	for _, index := range rows {
		if managedMetadataIndexName.MatchString(index.Name) {
			indexes[index.Name] = index
		}
	}
	return indexes, nil
}

func quoteMetadataIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func dropManagedMetadataIndex(db *gorm.DB, index managedMetadataIndex) error {
	if !managedMetadataIndexName.MatchString(index.Name) {
		return fmt.Errorf("refusing to remove unmanaged index")
	}
	name := quoteMetadataIdentifier(index.Name)
	concurrently := ""
	if db.Dialector.Name() == "postgres" {
		concurrently = " CONCURRENTLY"
		name = quoteMetadataIdentifier(index.SchemaName) + "." + name
	}
	return db.Exec("DROP INDEX" + concurrently + " " + name).Error
}

// Install before starting HTTP serving so scoped context copies share the
// manager and its status. The worker follows the server's shutdown context.
func (ctx *MahresourcesContext) StartMetadataIndexer(workerCtx context.Context) {
	ctx.metadataIndexer = NewMetadataIndexer(ctx.db)
	go ctx.metadataIndexer.Run(workerCtx)
}

func (ctx *MahresourcesContext) MetadataIndexStatus() contracts.MetadataIndexStatus {
	if ctx.metadataIndexer == nil {
		return contracts.MetadataIndexStatus{State: "unavailable", Detail: "Metadata index worker is not running"}
	}
	return ctx.metadataIndexer.Status()
}

// A physical index is shared by all carriers requesting the same entity/key/kind.
// Removing a declaration or deleting a carrier only retires an index when its
// last requester disappears. Read every carrier before making any DDL changes.
func collectCategoryMetadataIndexes(db *gorm.DB) ([]mrql.MetadataIndex, error) {
	unique := map[mrql.MetadataIndex]bool{}
	for _, carrier := range []struct{ table, entity string }{{"resource_categories", "resource"}, {"categories", "group"}, {"note_types", "note"}} {
		var rows []struct {
			ID              uint
			MetadataIndexes string
		}
		if err := db.Table(carrier.table).Select("id, metadata_indexes").Where("metadata_indexes IS NOT NULL AND metadata_indexes <> '' AND metadata_indexes <> '[]'").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			indexes, err := mrql.ParseMetadataIndexKeys(row.MetadataIndexes, carrier.entity)
			if err != nil {
				return nil, fmt.Errorf("%s %d: %w", carrier.table, row.ID, err)
			}
			for _, index := range indexes {
				unique[index] = true
			}
		}
	}
	indexes := make([]mrql.MetadataIndex, 0, len(unique))
	for index := range unique {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name() < indexes[j].Name() })
	return indexes, nil
}
