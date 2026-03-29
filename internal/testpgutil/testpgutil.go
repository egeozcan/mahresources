//go:build postgres

package testpgutil

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Container holds a running Postgres testcontainer and its admin DSN.
type Container struct {
	tc       *pgmodule.PostgresContainer
	AdminDSN string
}

// StartContainer starts a Postgres testcontainer. Call once per test package.
func StartContainer(ctx context.Context) (*Container, error) {
	pgContainer, err := pgmodule.Run(ctx,
		"postgres:16-alpine",
		pgmodule.WithDatabase("test"),
		pgmodule.WithUsername("test"),
		pgmodule.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	return &Container{tc: pgContainer, AdminDSN: dsn}, nil
}

// Stop terminates the container.
func (c *Container) Stop(ctx context.Context) error {
	if c.tc != nil {
		return c.tc.Terminate(ctx)
	}
	return nil
}

// DSN returns the admin DSN.
func (c *Container) DSN() string {
	return c.AdminDSN
}

// CreateTestDB creates a fresh database for a single test and returns a GORM
// connection. The database is dropped in t.Cleanup.
func (c *Container) CreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), rand.Intn(100000))

	adminDB, err := gorm.Open(pgdriver.Open(c.AdminDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to admin DB: %v", err)
	}
	if err := adminDB.Exec("CREATE DATABASE " + dbName).Error; err != nil {
		t.Fatalf("failed to create test database %s: %v", dbName, err)
	}
	sqlAdmin, _ := adminDB.DB()
	sqlAdmin.Close()

	testDSN := replaceDatabaseInDSN(c.AdminDSN, dbName)

	db, err := gorm.Open(pgdriver.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to test database %s: %v", dbName, err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()

		cleanupDB, err := gorm.Open(pgdriver.Open(c.AdminDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err == nil {
			cleanupDB.Exec("DROP DATABASE IF EXISTS " + dbName)
			sqlCleanup, _ := cleanupDB.DB()
			sqlCleanup.Close()
		}
	})

	return db
}

func replaceDatabaseInDSN(dsn, newDB string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	u.Path = "/" + newDB
	return u.String()
}
