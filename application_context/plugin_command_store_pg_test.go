//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/plugin_commands"
)

func newPluginCommandStorePGContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	require.NoError(t, db.AutoMigrate(
		&models.PluginCommandRun{}, &models.PluginCommandRunOutput{},
		&models.PluginCommandImport{}, &models.PluginCommandImportMap{},
	))
	readOnly, err := sqlx.Connect("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readOnly.Close() })
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{
		DbType: constants.DbTypePosgres, AuthEnabled: true,
	})
}

func TestPluginCommandRunStartPGHasExactlyOneWinner(t *testing.T) {
	ctx := newPluginCommandStorePGContext(t)
	now := time.Now().UTC()
	owner := uint(7)
	require.NoError(t, ctx.CreateRun(testRun("pg-start", &owner, false, now), testOutput("pg-start", now)))

	const racers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	wins := make([]bool, racers)
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			wins[i], errs[i] = ctx.MarkRunRunning("pg-start", now.Add(time.Duration(i)*time.Millisecond))
		}(i)
	}
	close(start)
	wg.Wait()
	winnerCount := 0
	for i := range wins {
		require.NoError(t, errs[i])
		if wins[i] {
			winnerCount++
		}
	}
	require.Equal(t, 1, winnerCount)
}

func TestPluginCommandRunFinishPGHasExactlyOneWinner(t *testing.T) {
	ctx := newPluginCommandStorePGContext(t)
	now := time.Now().UTC()
	owner := uint(8)
	require.NoError(t, ctx.CreateRun(testRun("pg-finish", &owner, false, now), testOutput("pg-finish", now)))
	won, err := ctx.MarkRunRunning("pg-finish", now)
	require.NoError(t, err)
	require.True(t, won)

	const racers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	wins := make([]bool, racers)
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			wins[i], errs[i] = ctx.FinishRun("pg-finish", plugin_commands.RunFinish{
				Status: plugin_commands.RunStatusSucceeded, OutputTail: fmt.Sprintf("winner-%d", i), FinishedAt: now.Add(time.Second),
			})
		}(i)
	}
	close(start)
	wg.Wait()
	winnerCount := 0
	for i := range wins {
		require.NoError(t, errs[i])
		if wins[i] {
			winnerCount++
		}
	}
	require.Equal(t, 1, winnerCount)
}

func pluginCommandCallbackTable(db *gorm.DB) string {
	if db.Statement.Schema != nil {
		return db.Statement.Schema.Table
	}
	return db.Statement.Table
}

func TestPluginCommandImportRecoveryPGDoesNotOverwriteConcurrentSuccess(t *testing.T) {
	ctx := newPluginCommandStorePGContext(t)
	now := time.Now().UTC()
	owner := uint(9)
	require.NoError(t, ctx.CreateRun(testRun("pg-recovery", &owner, false, now), testOutput("pg-recovery", now)))
	_, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{
		ImportID: "pg-recovery-import", RunID: "pg-recovery", FileName: "result.bin",
		PluginGeneration: 1, CreatedByUserID: &owner, CreatedAt: now,
	})
	require.NoError(t, err)
	won, err := ctx.MarkImportRunning("pg-recovery-import", now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)

	finishUpdatedClaim := make(chan struct{})
	releaseFinish := make(chan struct{})
	var pauseFinish sync.Once
	const pauseCallback = "test:pause_plugin_command_import_finish"
	require.NoError(t, ctx.db.Callback().Update().After("gorm:update").Register(pauseCallback, func(db *gorm.DB) {
		if pluginCommandCallbackTable(db) != "plugin_command_imports" {
			return
		}
		pauseFinish.Do(func() {
			close(finishUpdatedClaim)
			<-releaseFinish
		})
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(pauseCallback) })

	recoveryQueryStarted := make(chan struct{})
	recoveryQueryFinished := make(chan struct{})
	var queryStarted, queryFinished sync.Once
	const beforeQueryCallback = "test:observe_plugin_command_recovery_query_start"
	const afterQueryCallback = "test:observe_plugin_command_recovery_query_finish"
	require.NoError(t, ctx.db.Callback().Query().Before("gorm:query").Register(beforeQueryCallback, func(db *gorm.DB) {
		if pluginCommandCallbackTable(db) == "plugin_command_imports" {
			queryStarted.Do(func() { close(recoveryQueryStarted) })
		}
	}))
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(afterQueryCallback, func(db *gorm.DB) {
		if pluginCommandCallbackTable(db) == "plugin_command_imports" {
			queryFinished.Do(func() { close(recoveryQueryFinished) })
		}
	}))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Query().Remove(beforeQueryCallback)
		_ = ctx.db.Callback().Query().Remove(afterQueryCallback)
	})

	resourceID := uint(77)
	finishDone := make(chan struct{})
	var finishWon bool
	var finishErr error
	go func() {
		defer close(finishDone)
		finishWon, finishErr = ctx.FinishImport("pg-recovery-import", plugin_commands.ImportFinish{
			Status: plugin_commands.ImportStatusSucceeded, ResourceID: &resourceID,
			FinishedAt: now.Add(2 * time.Second),
		})
	}()
	<-finishUpdatedClaim

	recoveryDone := make(chan error, 1)
	go func() { recoveryDone <- ctx.InterruptNonterminalImports(now.Add(3 * time.Second)) }()
	<-recoveryQueryStarted
	select {
	case <-recoveryQueryFinished:
		// Without a row lock recovery observes the old running version here,
		// before the successful finish commits.
	case <-time.After(250 * time.Millisecond):
		// With FOR UPDATE the read waits for the finisher's row lock.
	}
	close(releaseFinish)
	<-finishDone
	require.NoError(t, finishErr)
	require.True(t, finishWon)
	require.NoError(t, <-recoveryDone)

	var claim models.PluginCommandImport
	require.NoError(t, ctx.db.First(&claim, "id = ?", "pg-recovery-import").Error)
	require.Equal(t, plugin_commands.ImportStatusSucceeded, claim.Status)
	mapped, ok, err := ctx.ImportMap("pg-recovery", "result.bin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, plugin_commands.ImportStatusSucceeded, mapped.Status)
	require.NotNil(t, mapped.ResourceID)
	require.Equal(t, resourceID, *mapped.ResourceID)
}

func TestPluginCommandImportClaimPGSerializesTerminalReplacement(t *testing.T) {
	ctx := newPluginCommandStorePGContext(t)
	now := time.Now().UTC()
	owner := uint(10)
	require.NoError(t, ctx.CreateRun(testRun("pg-replace", &owner, false, now), testOutput("pg-replace", now)))
	_, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{
		ImportID: "pg-old-import", RunID: "pg-replace", FileName: "same.bin",
		PluginGeneration: 1, CreatedByUserID: &owner, CreatedAt: now,
	})
	require.NoError(t, err)
	won, err := ctx.FinishImport("pg-old-import", plugin_commands.ImportFinish{
		Status: plugin_commands.ImportStatusFailed, Error: "old failure", FinishedAt: now.Add(time.Second),
	})
	require.NoError(t, err)
	require.True(t, won)

	firstRead := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondRead := make(chan struct{})
	secondQueryStarted := make(chan struct{})
	var mapQueries atomic.Int32
	const beforeCallback = "test:observe_second_plugin_command_map_query"
	const afterCallback = "test:pause_first_plugin_command_map_query"
	require.NoError(t, ctx.db.Callback().Query().Before("gorm:query").Register(beforeCallback, func(db *gorm.DB) {
		if pluginCommandCallbackTable(db) != "plugin_command_import_maps" {
			return
		}
		if mapQueries.Load() >= 1 {
			select {
			case <-secondQueryStarted:
			default:
				close(secondQueryStarted)
			}
		}
	}))
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(afterCallback, func(db *gorm.DB) {
		if pluginCommandCallbackTable(db) != "plugin_command_import_maps" {
			return
		}
		switch mapQueries.Add(1) {
		case 1:
			close(firstRead)
			<-releaseFirst
		case 2:
			close(secondRead)
		}
	}))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Query().Remove(beforeCallback)
		_ = ctx.db.Callback().Query().Remove(afterCallback)
	})

	claim := func(id string) (plugin_commands.ImportClaimResult, error) {
		return ctx.ClaimImport(plugin_commands.ImportClaimRequest{
			ImportID: id, RunID: "pg-replace", FileName: "same.bin",
			PluginGeneration: 2, CreatedByUserID: &owner, CreatedAt: now.Add(2 * time.Second),
		})
	}
	results := make([]plugin_commands.ImportClaimResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0], errs[0] = claim("pg-replacement-a")
	}()
	<-firstRead
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[1], errs[1] = claim("pg-replacement-b")
	}()
	<-secondQueryStarted
	select {
	case <-secondRead:
		// This is reachable when the existing map row is read without FOR UPDATE.
	case <-time.After(250 * time.Millisecond):
		// The second read is blocked behind the first transaction's row lock.
	}
	close(releaseFirst)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	created := 0
	winner := ""
	for _, result := range results {
		if result.Created {
			created++
			winner = result.ImportID
		}
	}
	require.Equal(t, 1, created)
	require.NotEmpty(t, winner)
	for _, result := range results {
		require.Equal(t, winner, result.ImportID)
	}
	var pending int64
	require.NoError(t, ctx.db.Model(&models.PluginCommandImport{}).
		Where("run_id = ? AND status = ?", "pg-replace", plugin_commands.ImportStatusPending).Count(&pending).Error)
	require.Equal(t, int64(1), pending, "the losing replacement must not leave an orphan pending claim")
}

func TestPluginCommandImportClaimPGHasExactlyOneWinner(t *testing.T) {
	ctx := newPluginCommandStorePGContext(t)
	now := time.Now().UTC()
	owner := uint(9)
	require.NoError(t, ctx.CreateRun(testRun("pg-claim", &owner, false, now), testOutput("pg-claim", now)))

	const racers = 8
	start := make(chan struct{})
	results := make([]plugin_commands.ImportClaimResult, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = ctx.ClaimImport(plugin_commands.ImportClaimRequest{
				ImportID: fmt.Sprintf("pg-import-%d", i), RunID: "pg-claim", FileName: "same.bin",
				PluginGeneration: 1, CreatedByUserID: &owner, CreatedAt: now,
			})
		}(i)
	}
	close(start)
	wg.Wait()

	winner := ""
	wins := 0
	for i := range results {
		require.NoError(t, errs[i])
		if results[i].Created {
			wins++
			winner = results[i].ImportID
		}
	}
	require.Equal(t, 1, wins)
	for _, result := range results {
		require.Equal(t, winner, result.ImportID)
	}
}
