package mrqlbench

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type DisposablePostgres struct {
	container *pgmodule.PostgresContainer
	DSN       string
}

func StartDisposablePostgres(ctx context.Context) (*DisposablePostgres, error) {
	container, err := pgmodule.Run(ctx,
		"postgres:16-alpine",
		pgmodule.WithDatabase("mrql_benchmark"),
		pgmodule.WithUsername("benchmark"),
		pgmodule.WithPassword("benchmark"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		return nil, fmt.Errorf("start disposable postgres: %w", err)
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("postgres connection string: %w", err)
	}
	return &DisposablePostgres{container: container, DSN: dsn}, nil
}

func (p *DisposablePostgres) Close(ctx context.Context) error {
	if p == nil || p.container == nil {
		return nil
	}
	return p.container.Terminate(ctx)
}
