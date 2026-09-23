package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
)

func importCommandFactField(path string) (handle, field string) {
	clean := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(clean, "_imports/") {
		return "", ""
	}
	name := strings.TrimPrefix(clean, "_imports/")
	switch {
	case strings.HasSuffix(name, ".plan.json") && !strings.Contains(strings.TrimSuffix(name, ".plan.json"), "/"):
		return strings.TrimSuffix(name, ".plan.json"), "plan_available"
	case strings.HasSuffix(name, ".tar") && !strings.Contains(strings.TrimSuffix(name, ".tar"), "/"):
		return strings.TrimSuffix(name, ".tar"), "archive_available"
	default:
		return "", ""
	}
}

func (ctx *MahresourcesContext) noteImportFileChange(path string, available bool) error {
	if ctx == nil || ctx.JobService() == nil || ctx.db == nil {
		return nil
	}
	handle, field := importCommandFactField(path)
	if handle == "" {
		return nil
	}
	return setImportCommandFactField(ctx.db, handle, field, available)
}

func setImportCommandFactField(db *gorm.DB, handle, field string, available bool) error {
	if db == nil || strings.TrimSpace(handle) == "" {
		return nil
	}
	var fact models.JobImportCommandFact
	fact.ParseHandle = handle
	assignments := map[string]any{"updated_at": time.Now().UTC()}
	switch field {
	case "archive_available":
		fact.ArchiveAvailable = available
		assignments[field] = available
	case "plan_available":
		fact.PlanAvailable = available
		assignments[field] = available
	default:
		return fmt.Errorf("unknown import command fact field %q", field)
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "parse_handle"}},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&fact).Error
}

func setImportCommandAvailability(db *gorm.DB, handle string, archiveAvailable, planAvailable bool) error {
	if db == nil || strings.TrimSpace(handle) == "" {
		return nil
	}
	fact := models.JobImportCommandFact{
		ParseHandle:      handle,
		ArchiveAvailable: archiveAvailable,
		PlanAvailable:    planAvailable,
		UpdatedAt:        time.Now().UTC(),
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "parse_handle"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"archive_available", "plan_available", "updated_at",
		}),
	}).Create(&fact).Error
}

func importCommandAvailability(db *gorm.DB, handle string) (archiveAvailable, planAvailable bool, err error) {
	if db == nil || strings.TrimSpace(handle) == "" {
		return false, false, nil
	}
	var fact models.JobImportCommandFact
	err = db.Where("parse_handle = ?", handle).First(&fact).Error
	if err != nil {
		if errorsIsNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}
	return fact.ArchiveAvailable, fact.PlanAvailable, nil
}

func importParseSummaryOf(raw []byte) (importParseSummary, bool) {
	var summary importParseSummary
	if len(raw) == 0 || !json.Valid(raw) || json.Unmarshal(raw, &summary) != nil || summary.Handle == "" {
		return importParseSummary{}, false
	}
	return summary, true
}

func importApplySummaryOf(raw []byte) (importApplySummary, bool) {
	var summary importApplySummary
	if len(raw) == 0 || !json.Valid(raw) || json.Unmarshal(raw, &summary) != nil || summary.ParseHandle == "" {
		return importApplySummary{}, false
	}
	return summary, true
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func (ctx *MahresourcesContext) initializeImportCommandAvailability(handle string) error {
	if ctx == nil || ctx.JobService() == nil || ctx.db == nil {
		return nil
	}
	archiveAvailable, _ := afero.Exists(ctx.fs, importArchivePathFor(handle))
	planAvailable, _ := afero.Exists(ctx.fs, importPlanPathFor(handle))
	return setImportCommandAvailability(ctx.db, handle, archiveAvailable, planAvailable)
}

// ReconcileImportCommandAvailability repairs the indexed artifact facts used by
// import Retry commands. Startup calls it after SetJobService has registered the
// import Kinds, and before plugin activation or dispatch can expose these Jobs.
// It reconciles existing facts against the filesystem, then backfills handles in
// existing import Jobs that predate the fact table. The bounded keyset scans run
// only at startup, never while a list or summary is being selected.
func (ctx *MahresourcesContext) ReconcileImportCommandAvailability() error {
	if ctx == nil || ctx.JobService() == nil || ctx.db == nil {
		return fmt.Errorf("import command availability reconciliation requires a database and installed Job service")
	}
	if err := ctx.reconcileImportCommandFacts(); err != nil {
		return err
	}
	return ctx.backfillImportCommandFactsFromJobs()
}

func (ctx *MahresourcesContext) reconcileImportCommandFacts() error {
	if ctx == nil || ctx.db == nil {
		return nil
	}
	const batchSize = 500
	lastHandle := ""
	for {
		var facts []models.JobImportCommandFact
		query := ctx.db.Order("parse_handle ASC").Limit(batchSize)
		if lastHandle != "" {
			query = query.Where("parse_handle > ?", lastHandle)
		}
		if err := query.Find(&facts).Error; err != nil {
			return err
		}
		if len(facts) == 0 {
			return nil
		}
		changed := make([]models.JobImportCommandFact, 0, len(facts))
		for _, fact := range facts {
			archiveAvailable, err := afero.Exists(ctx.fs, importArchivePathFor(fact.ParseHandle))
			if err != nil {
				return fmt.Errorf("check archive for import %q: %w", fact.ParseHandle, err)
			}
			planAvailable, err := afero.Exists(ctx.fs, importPlanPathFor(fact.ParseHandle))
			if err != nil {
				return fmt.Errorf("check plan for import %q: %w", fact.ParseHandle, err)
			}
			if archiveAvailable != fact.ArchiveAvailable || planAvailable != fact.PlanAvailable {
				changed = append(changed, models.JobImportCommandFact{
					ParseHandle: fact.ParseHandle, ArchiveAvailable: archiveAvailable,
					PlanAvailable: planAvailable, UpdatedAt: time.Now().UTC(),
				})
			}
			lastHandle = fact.ParseHandle
		}
		if len(changed) > 0 {
			// Four bound values per row keep each write under SQLite's 999-parameter
			// ceiling while reducing a million stale facts to bounded batches.
			if err := ctx.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "parse_handle"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"archive_available", "plan_available", "updated_at",
				}),
			}).CreateInBatches(&changed, 200).Error; err != nil {
				return err
			}
		}
	}
}

func importCommandHandleForJob(job models.Job) string {
	switch job.Kind {
	case JobKindGroupImportParse:
		summary, ok := importParseSummaryOf(job.Summary)
		if ok {
			return summary.Handle
		}
	case JobKindGroupImportApply:
		summary, ok := importApplySummaryOf(job.Summary)
		if ok {
			return summary.ParseHandle
		}
	}
	return ""
}

// backfillImportCommandFactsFromJobs finds missing parse handles from the safe,
// indexed Job summaries. It deliberately does not open replay envelopes: parse
// and apply summaries already carry the handle the selector needs, and decrypting
// each Job at startup would make this repair unbounded in both CPU and key access.
func (ctx *MahresourcesContext) backfillImportCommandFactsFromJobs() error {
	const batchSize = 500
	lastID := ""
	for {
		var page []models.Job
		query := ctx.db.Model(&models.Job{}).
			Select("id, kind, summary").
			Where("(kind = ? AND kind_version = ?) OR (kind = ? AND kind_version = ?)",
				JobKindGroupImportParse, jobImportKindVersion,
				JobKindGroupImportApply, jobImportKindVersion).
			Order("id ASC").Limit(batchSize)
		if lastID != "" {
			query = query.Where("id > ?", lastID)
		}
		if err := query.Find(&page).Error; err != nil {
			return fmt.Errorf("read import Jobs for command fact backfill: %w", err)
		}
		if len(page) == 0 {
			return nil
		}
		lastID = page[len(page)-1].ID

		handles := make([]string, 0, len(page))
		seen := make(map[string]struct{}, len(page))
		for _, job := range page {
			handle := importCommandHandleForJob(job)
			if handle == "" {
				continue
			}
			if _, duplicate := seen[handle]; duplicate {
				continue
			}
			seen[handle] = struct{}{}
			handles = append(handles, handle)
		}
		if len(handles) == 0 {
			continue
		}

		var present []string
		if err := ctx.db.Model(&models.JobImportCommandFact{}).
			Where("parse_handle IN ?", handles).Pluck("parse_handle", &present).Error; err != nil {
			return fmt.Errorf("read existing import command facts: %w", err)
		}
		known := make(map[string]struct{}, len(present))
		for _, handle := range present {
			known[handle] = struct{}{}
		}
		missing := make([]models.JobImportCommandFact, 0, len(handles)-len(present))
		for _, handle := range handles {
			if _, ok := known[handle]; ok {
				continue
			}
			archiveAvailable, err := afero.Exists(ctx.fs, importArchivePathFor(handle))
			if err != nil {
				return fmt.Errorf("check archive for import %q during backfill: %w", handle, err)
			}
			planAvailable, err := afero.Exists(ctx.fs, importPlanPathFor(handle))
			if err != nil {
				return fmt.Errorf("check plan for import %q during backfill: %w", handle, err)
			}
			missing = append(missing, models.JobImportCommandFact{
				ParseHandle: handle, ArchiveAvailable: archiveAvailable,
				PlanAvailable: planAvailable, UpdatedAt: time.Now().UTC(),
			})
		}
		if len(missing) > 0 {
			if err := ctx.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "parse_handle"}},
				DoNothing: true,
			}).CreateInBatches(&missing, 200).Error; err != nil {
				return fmt.Errorf("write backfilled import command facts: %w", err)
			}
		}
	}
}

type importFactTrackingFS struct {
	afero.Fs
	ctx *MahresourcesContext
}

func (f importFactTrackingFS) OpenFile(name string, flag int, perm fs.FileMode) (afero.File, error) {
	file, err := f.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		if _, field := importCommandFactField(name); field != "" {
			// A writer can truncate or partially replace an artifact. Hide its
			// availability until the caller has successfully closed it.
			if err := f.ctx.noteImportFileChange(name, false); err != nil {
				_ = file.Close()
				return nil, err
			}
			return trackedImportFactFile{File: file, fs: f.Fs, ctx: f.ctx, name: name}, nil
		}
	}
	return file, nil
}

func (f importFactTrackingFS) Create(name string) (afero.File, error) {
	file, err := f.Fs.Create(name)
	if err != nil {
		return nil, err
	}
	if _, field := importCommandFactField(name); field != "" {
		if err := f.ctx.noteImportFileChange(name, false); err != nil {
			_ = file.Close()
			return nil, err
		}
		return trackedImportFactFile{File: file, fs: f.Fs, ctx: f.ctx, name: name}, nil
	}
	return file, nil
}

type trackedImportFactFile struct {
	afero.File
	fs   afero.Fs
	ctx  *MahresourcesContext
	name string
}

func (f trackedImportFactFile) Close() error {
	if err := f.File.Close(); err != nil {
		return err
	}
	exists, err := afero.Exists(f.fs, f.name)
	if err != nil {
		return err
	}
	return f.ctx.noteImportFileChange(f.name, exists)
}

func (f importFactTrackingFS) Rename(oldname, newname string) error {
	if err := f.Fs.Rename(oldname, newname); err != nil {
		return err
	}
	if err := f.ctx.noteImportFileChange(oldname, false); err != nil {
		return err
	}
	return f.ctx.noteImportFileChange(newname, true)
}

func (f importFactTrackingFS) Remove(name string) error {
	if err := f.Fs.Remove(name); err != nil {
		return err
	}
	return f.ctx.noteImportFileChange(name, false)
}

func (f importFactTrackingFS) RemoveAll(path string) error {
	if err := f.Fs.RemoveAll(path); err != nil {
		return err
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "_imports" || strings.HasPrefix(clean, "_imports/") {
		return f.ctx.reconcileImportCommandFacts()
	}
	return f.ctx.noteImportFileChange(path, false)
}

func (ctx *MahresourcesContext) defaultFsWithImportFacts() afero.Fs {
	if ctx == nil || ctx.fs == nil {
		return nil
	}
	return importFactTrackingFS{Fs: ctx.fs, ctx: ctx}
}
