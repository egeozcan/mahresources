//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

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
