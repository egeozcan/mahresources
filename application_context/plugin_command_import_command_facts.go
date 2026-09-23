package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_commands"
	"mahresources/plugin_system"
)

const pluginCommandImportRetryKey = "retry-import"

type pluginCommandImportFileProbe interface {
	HasRegularFile(pluginName, runID, name string) bool
}

type pluginCommandImportFactFields struct {
	validated bool
	seriesID  uint
	groups    []models.PluginCommandImportCommandFactGroup
}

func pluginCommandImportFactFieldsJSON(encoded string) pluginCommandImportFactFields {
	var fields plugin_commands.ResourceFields
	if encoded == "" || !json.Valid([]byte(encoded)) || json.Unmarshal([]byte(encoded), &fields) != nil {
		return pluginCommandImportFactFields{}
	}
	// The raw FieldsJSON is limited to 64 KiB by SubmitImport. Keep selector
	// facts similarly bounded so one corrupt legacy row cannot create an
	// unbounded reconciliation write.
	if len(fields.GroupIDs) > 4096 {
		return pluginCommandImportFactFields{}
	}
	result := pluginCommandImportFactFields{validated: true, seriesID: fields.SeriesID}
	seen := make(map[uint]struct{}, len(fields.GroupIDs))
	for _, id := range fields.GroupIDs {
		if id == 0 {
			return pluginCommandImportFactFields{}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result.groups = append(result.groups, models.PluginCommandImportCommandFactGroup{
			GroupID: id,
		})
	}
	sort.Slice(result.groups, func(i, j int) bool { return result.groups[i].GroupID < result.groups[j].GroupID })
	return result
}

func (ctx *MahresourcesContext) pluginCommandImportFieldsJSON(db *gorm.DB, source models.PluginCommandImport) (string, bool) {
	// Read the durable writer fence before looking at compatibility plaintext.
	// Once epoch two is active, a nonempty legacy projection is stale data, not a
	// fallback for an unavailable or purged canonical envelope.
	retired, err := pluginCommandInputsRetired(db)
	if err != nil {
		return "", false
	}
	if source.JobID == "" {
		if !retired && source.FieldsJSON != "" {
			return source.FieldsJSON, true
		}
		return "", false
	}

	// Purge markers and elapsed deadlines take precedence over every source copy,
	// including a pre-retirement compatibility projection. Otherwise startup
	// reconciliation could recreate a fact that Forget or expiry just removed.
	var envelope models.JobReplayEnvelope
	envelopeErr := db.Where("job_id = ?", source.JobID).First(&envelope).Error
	envelopeFound := envelopeErr == nil
	if envelopeErr != nil && !errors.Is(envelopeErr, gorm.ErrRecordNotFound) {
		return "", false
	}
	if envelopeFound && (envelope.PurgedAt != nil || len(envelope.Ciphertext) == 0 ||
		(envelope.ExpiresAt != nil && !envelope.ExpiresAt.After(time.Now().UTC()))) {
		return "", false
	}
	if !retired && source.FieldsJSON != "" {
		return source.FieldsJSON, true
	}
	if ctx == nil || ctx.JobService() == nil || !envelopeFound {
		return "", false
	}
	opened, err := ctx.JobService().OpenReplay(ctx.jobDepsWithDB(db), jobs.Access{Administrator: true}, source.JobID)
	if err != nil {
		return "", false
	}
	var input pluginCommandImportReplayInput
	if json.Unmarshal(opened.Input, &input) != nil || input.RunID != source.RunID ||
		input.FileName != source.FileName || input.FieldsJSON == "" {
		return "", false
	}
	return input.FieldsJSON, true
}

func setPluginCommandImportFactTx(tx *gorm.DB, source models.PluginCommandImport, fieldsJSON string, fileAvailable bool, now time.Time) error {
	if tx == nil || source.JobID == "" || source.ID == "" || source.RunID == "" || source.FileName == "" {
		return nil
	}
	fields := pluginCommandImportFactFieldsJSON(fieldsJSON)
	fact := models.PluginCommandImportCommandFact{
		JobID: source.JobID, ImportID: source.ID, RunID: source.RunID, FileName: source.FileName,
		FieldsValidated: fields.validated, ExchangeFileAvailable: fileAvailable,
		SeriesID: fields.seriesID, GroupCount: len(fields.groups), UpdatedAt: now.UTC(),
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"import_id", "run_id", "file_name", "fields_validated", "exchange_file_available", "series_id", "group_count", "updated_at",
		}),
	}).Create(&fact).Error; err != nil {
		return err
	}
	if err := tx.Where("import_id = ?", source.ID).Delete(&models.PluginCommandImportCommandFactGroup{}).Error; err != nil {
		return err
	}
	for i := range fields.groups {
		fields.groups[i].ImportID = source.ID
	}
	if len(fields.groups) == 0 {
		return nil
	}
	return tx.CreateInBatches(&fields.groups, 200).Error
}

// purgePluginCommandImportReplayFactsTx removes the Kind-owned projection of
// replay input in the caller's lifecycle transaction. Source import rows may
// outlive a forgotten envelope or retained Job, so reconciliation also treats
// the canonical envelope as authoritative before it can rebuild these rows.
func purgePluginCommandImportReplayFactsTx(tx *gorm.DB, jobIDs []string) error {
	if tx == nil || len(jobIDs) == 0 {
		return nil
	}
	hasFacts := tx.Migrator().HasTable(&models.PluginCommandImportCommandFact{})
	hasGroups := tx.Migrator().HasTable(&models.PluginCommandImportCommandFactGroup{})
	hasSources := tx.Migrator().HasTable(&models.PluginCommandImport{})
	if hasGroups {
		var factImports, sourceImports *gorm.DB
		if hasFacts {
			factImports = tx.Model(&models.PluginCommandImportCommandFact{}).
				Select("import_id").Where("job_id IN ?", jobIDs)
		}
		if hasSources {
			sourceImports = tx.Model(&models.PluginCommandImport{}).
				Select("id").Where("job_id IN ?", jobIDs)
		}
		query := tx
		switch {
		case factImports != nil && sourceImports != nil:
			query = query.Where("import_id IN (?) OR import_id IN (?)", factImports, sourceImports)
		case factImports != nil:
			query = query.Where("import_id IN (?)", factImports)
		case sourceImports != nil:
			query = query.Where("import_id IN (?)", sourceImports)
		default:
			query = nil
		}
		if query != nil {
			if err := query.Delete(&models.PluginCommandImportCommandFactGroup{}).Error; err != nil {
				return err
			}
		}
	}
	if hasFacts {
		return tx.Where("job_id IN ?", jobIDs).Delete(&models.PluginCommandImportCommandFact{}).Error
	}
	return nil
}

func purgePluginCommandImportFactsForSourcesTx(tx *gorm.DB, jobIDs, importIDs []string) error {
	if tx == nil {
		return nil
	}
	if len(importIDs) != 0 {
		if err := tx.Where("import_id IN ?", importIDs).
			Delete(&models.PluginCommandImportCommandFactGroup{}).Error; err != nil {
			return err
		}
	}
	return purgePluginCommandImportReplayFactsTx(tx, jobIDs)
}

func updatePluginCommandImportFactAvailabilityTx(tx *gorm.DB, runID, fileName string, available bool, now time.Time) error {
	query := tx.Model(&models.PluginCommandImportCommandFact{}).Where("run_id = ?", runID)
	if fileName != "" {
		query = query.Where("file_name = ?", fileName)
	}
	return query.Updates(map[string]any{"exchange_file_available": available, "updated_at": now.UTC()}).Error
}

func (ctx *MahresourcesContext) markPluginCommandImportFileUnavailable(runID, fileName string) error {
	return ctx.setPluginCommandImportFileAvailability(runID, fileName, false)
}

func (ctx *MahresourcesContext) setPluginCommandImportFileAvailability(runID, fileName string, available bool) error {
	if ctx == nil || ctx.db == nil {
		return fmt.Errorf("plugin command import fact database is unavailable")
	}
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		return updatePluginCommandImportFactAvailabilityTx(tx, runID, fileName, available, time.Now().UTC())
	})
}

func (ctx *MahresourcesContext) failClosedPluginCommandImportRetryFacts() error {
	if ctx == nil || ctx.db == nil {
		return fmt.Errorf("plugin command import fact database is unavailable")
	}
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		return tx.Model(&models.PluginCommandImportCommandFact{}).
			Where("exchange_file_available = ?", true).
			Updates(map[string]any{"exchange_file_available": false, "updated_at": time.Now().UTC()}).Error
	})
}

func (ctx *MahresourcesContext) pluginCommandImportRetryFenceToken() (string, bool) {
	if ctx == nil || !ctx.pluginCommandFenceOwned() || ctx.pluginCommandController == nil {
		return "", false
	}
	ctx.pluginCommandController.mu.Lock()
	token := ctx.pluginCommandController.dbFence
	ctx.pluginCommandController.mu.Unlock()
	return token, token != ""
}

// ReconcilePluginCommandImportRetryFacts repairs the indexed facts after
// command-runtime recovery. It runs only during startup, after the fenced
// runtime has recovered its source rows and before this process exposes command
// selectors. Each page and write is bounded; filesystem probes are one per
// import because exchange presence cannot be answered by SQL.
func (ctx *MahresourcesContext) ReconcilePluginCommandImportRetryFacts() error {
	if ctx == nil || ctx.db == nil || ctx.JobService() == nil {
		return fmt.Errorf("plugin command import fact reconciliation requires a database and installed Job service")
	}
	active, err := ctx.pluginCommandActive()
	if err != nil {
		return fmt.Errorf("plugin command runtime is unavailable for import fact reconciliation: %w", err)
	}
	probe, ok := active.exchange.(pluginCommandImportFileProbe)
	if !ok || probe == nil {
		return fmt.Errorf("plugin command exchange cannot verify regular files")
	}

	const batchSize = 100
	lastID := ""
	for {
		var imports []models.PluginCommandImport
		query := ctx.db.Model(&models.PluginCommandImport{}).Where("job_id <> ''").Order("id ASC").Limit(batchSize)
		if lastID != "" {
			query = query.Where("id > ?", lastID)
		}
		if err := query.Find(&imports).Error; err != nil {
			return fmt.Errorf("read plugin command imports for fact reconciliation: %w", err)
		}
		if len(imports) == 0 {
			return nil
		}
		lastID = imports[len(imports)-1].ID

		runIDs := make([]string, 0, len(imports))
		for _, source := range imports {
			runIDs = append(runIDs, source.RunID)
		}
		var runs []models.PluginCommandRun
		if err := ctx.db.Select("id, plugin_name").Where("id IN ?", runIDs).Find(&runs).Error; err != nil {
			return fmt.Errorf("read parent command runs for import fact reconciliation: %w", err)
		}
		runByID := make(map[string]models.PluginCommandRun, len(runs))
		for _, run := range runs {
			runByID[run.ID] = run
		}
		jobIDs := make([]string, 0, len(imports))
		for _, source := range imports {
			jobIDs = append(jobIDs, source.JobID)
		}
		var presentJobIDs []string
		if err := ctx.db.Model(&models.Job{}).Where("id IN ?", jobIDs).Pluck("id", &presentJobIDs).Error; err != nil {
			return fmt.Errorf("read Jobs for plugin command import fact reconciliation: %w", err)
		}
		presentJobIDSet := make(map[string]struct{}, len(presentJobIDs))
		for _, jobID := range presentJobIDs {
			presentJobIDSet[jobID] = struct{}{}
		}

		facts := make([]models.PluginCommandImportCommandFact, 0, len(imports))
		groups := make([]models.PluginCommandImportCommandFactGroup, 0)
		staleJobIDs := make([]string, 0)
		staleImportIDs := make([]string, 0)
		for _, source := range imports {
			if _, exists := presentJobIDSet[source.JobID]; !exists {
				staleJobIDs = append(staleJobIDs, source.JobID)
				staleImportIDs = append(staleImportIDs, source.ID)
				continue
			}
			fieldsJSON, found := ctx.pluginCommandImportFieldsJSON(ctx.db, source)
			fieldFacts := pluginCommandImportFactFieldsJSON(fieldsJSON)
			if !found || !fieldFacts.validated {
				// Missing, retired, forgotten, expired, or invalid input has no
				// retry fact. In particular, do not leave a blank fact that a later
				// pass could mistake for a source worth reconstructing.
				staleJobIDs = append(staleJobIDs, source.JobID)
				staleImportIDs = append(staleImportIDs, source.ID)
				continue
			}
			run, runFound := runByID[source.RunID]
			fileAvailable := false
			if runFound && source.FileName != "" {
				fileAvailable = probe.HasRegularFile(run.PluginName, source.RunID, source.FileName)
			}
			facts = append(facts, models.PluginCommandImportCommandFact{
				JobID: source.JobID, ImportID: source.ID, RunID: source.RunID, FileName: source.FileName,
				FieldsValidated: true, ExchangeFileAvailable: fileAvailable,
				SeriesID: fieldFacts.seriesID, GroupCount: len(fieldFacts.groups), UpdatedAt: time.Now().UTC(),
			})
			for _, group := range fieldFacts.groups {
				group.ImportID = source.ID
				groups = append(groups, group)
			}
		}

		if err := ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
				return err
			}
			if len(facts) != 0 {
				if err := tx.Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "job_id"}},
					DoUpdates: clause.AssignmentColumns([]string{
						"import_id", "run_id", "file_name", "fields_validated", "exchange_file_available", "series_id", "group_count", "updated_at",
					}),
				}).CreateInBatches(&facts, 100).Error; err != nil {
					return err
				}
			}
			importIDs := make([]string, len(imports))
			for i := range imports {
				importIDs[i] = imports[i].ID
			}
			if err := tx.Where("import_id IN ?", importIDs).Delete(&models.PluginCommandImportCommandFactGroup{}).Error; err != nil {
				return err
			}
			if err := purgePluginCommandImportFactsForSourcesTx(tx, staleJobIDs, staleImportIDs); err != nil {
				return err
			}
			for start := 0; start < len(groups); start += 200 {
				end := min(start+200, len(groups))
				if err := tx.Create(groups[start:end]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("write plugin command import facts: %w", err)
		}
	}
}

// pluginCommandImportRetrySelection is shared by the detail advertisement and
// database selector. Every predicate that can be expressed from current
// authority is in this SQL query, while the two non-SQL facts come from the
// durable projection above.
func pluginCommandImportRetrySelection(db, base *gorm.DB, fenceToken string, pluginNames, replayKeyIDs []string, replayNow time.Time) *gorm.DB {
	if base == nil {
		return nil
	}
	if db == nil || fenceToken == "" {
		return base.Where("1 = 0").Select("jobs.id")
	}
	if len(pluginNames) == 0 || len(replayKeyIDs) == 0 {
		return base.Where("1 = 0").Select("jobs.id")
	}
	return base.Where(`EXISTS (
		SELECT 1
		FROM plugin_command_imports pci
		JOIN plugin_command_import_command_facts pcf
		  ON pcf.job_id = pci.job_id
		 AND pcf.import_id = pci.id
		 AND pcf.run_id = pci.run_id
		 AND pcf.file_name = pci.file_name
		JOIN plugin_command_import_maps pcm
		  ON pcm.run_id = pci.run_id
		 AND pcm.file_name = pci.file_name
		 AND pcm.import_id = pci.id
		JOIN plugin_command_runs pcr ON pcr.id = pci.run_id
		JOIN users actor ON actor.id = pci.created_by_user_id
		JOIN plugin_states ps ON ps.plugin_name = pcr.plugin_name AND ps.enabled = ?
		WHERE pci.job_id = jobs.id
		  AND jobs.state IN ?
		  AND pci.status IN ?
		  AND pci.created_by_user_id IS NOT NULL
		  AND actor.disabled = ?
		  AND actor.role IN ?
		  AND pcr.plugin_name IN ?
		  AND pcm.status = pci.status
		  AND pcm.resource_id IS NULL
		  AND pcr.status = ?
		  AND pcr.output_unverified = ?
		  AND pcf.fields_validated = ?
		  AND pcf.exchange_file_available = ?
		  AND (SELECT COUNT(*) FROM plugin_command_import_command_fact_groups pfg WHERE pfg.import_id = pcf.import_id) = pcf.group_count
		  AND jobs.replay_class = ?
		  AND EXISTS (
			SELECT 1 FROM job_replay_envelopes pre
			WHERE pre.job_id = jobs.id
			  AND pre.kind = ? AND pre.kind_version = ?
			  AND pre.purged_at IS NULL
			  AND (pre.expires_at IS NULL OR pre.expires_at > ?)
			  AND pre.key_id IN ?
		  )
		  AND (pcf.series_id = 0 OR EXISTS (SELECT 1 FROM series s WHERE s.id = pcf.series_id))
		  AND NOT EXISTS (
			SELECT 1
			FROM plugin_command_import_command_fact_groups pfg
			LEFT JOIN groups g ON g.id = pfg.group_id
			WHERE pfg.import_id = pcf.import_id AND g.id IS NULL
		  )
		  AND (
			actor.role = ? OR actor.scope_group_id IS NULL OR NOT EXISTS (
				SELECT 1 FROM plugin_command_import_command_fact_groups pfg
				WHERE pfg.import_id = pcf.import_id
				  AND NOT EXISTS (
					WITH RECURSIVE actor_group_scope(id) AS (
						SELECT actor.scope_group_id
						UNION
						SELECT child.id FROM groups child JOIN actor_group_scope parent ON child.owner_id = parent.id
					)
					SELECT 1 FROM actor_group_scope WHERE id = pfg.group_id
				  )
			)
		  )
		  AND EXISTS (
			SELECT 1 FROM job_runtime_fences jrf
			WHERE jrf.key = ? AND jrf.token = ? AND pcf.updated_at >= jrf.acquired_at
		  )
	)`,
		true,
		[]jobs.State{jobs.StateFailed, jobs.StateCancelled, jobs.StateInterrupted},
		[]string{plugin_commands.ImportStatusFailed, plugin_commands.ImportStatusCancelled, plugin_commands.ImportStatusInterrupted},
		false,
		[]models.Role{models.RoleAdmin, models.RoleEditor, models.RoleUser},
		pluginNames,
		plugin_commands.RunStatusSucceeded,
		false,
		true,
		true,
		jobs.ReplayClassReplayable,
		JobKindPluginCommandImport,
		jobPluginCommandVersion,
		replayNow.UTC(),
		replayKeyIDs,
		models.RoleAdmin,
		pluginCommandRuntimeFenceKey,
		fenceToken,
	).Select("jobs.id")
}

func (a *pluginCommandJobAdapter) pluginCommandImportRetryEligible(deps jobs.Deps, jobID string) (bool, error) {
	if a == nil || a.ctx == nil || deps.DB == nil {
		return false, nil
	}
	token, owned := a.ctx.pluginCommandImportRetryFenceToken()
	if !owned {
		return false, nil
	}
	pluginNames := a.ctx.pluginCommandImportRetryPluginNames()
	keys := pluginRetryReplayKeyring(deps)
	if len(pluginNames) == 0 || keys == nil {
		return false, nil
	}
	var count int64
	query := deps.DB.Model(&models.Job{}).Where("jobs.id = ? AND jobs.kind = ? AND jobs.kind_version = ?",
		jobID, JobKindPluginCommandImport, jobPluginCommandVersion)
	query = pluginCommandImportRetrySelection(deps.DB, query, token, pluginNames, keys.KeyIDs(), pluginRetryReplayNow(deps))
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 1, nil
}

func pluginRetryReplayKeyring(deps jobs.Deps) *jobs.Keyring {
	if deps.Replay == nil {
		return nil
	}
	return deps.Replay.Keys
}

func pluginRetryReplayNow(deps jobs.Deps) time.Time {
	if deps.Now != nil {
		return deps.Now().UTC()
	}
	return time.Now().UTC()
}

func (ctx *MahresourcesContext) pluginCommandImportRetryPluginNames() []string {
	if ctx == nil || ctx.PluginManager() == nil {
		return nil
	}
	var names []string
	for _, plugin := range ctx.PluginManager().Plugins() {
		caps := plugin.Manifest.Capabilities()
		if caps.Has(plugin_system.CapCommands) && caps.Has(plugin_system.CapDBWrite) {
			names = append(names, plugin.Name)
		}
	}
	return names
}
