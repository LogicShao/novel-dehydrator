package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates a new pgxpool connection pool from a database URL.
// The caller is responsible for closing the pool via pool.Close().
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db.New: parse config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("db.New: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db.New: ping: %w", err)
	}

	return pool, nil
}

// QueryRow executes a query expected to return at most one row.
// Always call Scan on the returned row.
func QueryRow(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) pgx.Row {
	return pool.QueryRow(ctx, sql, args...)
}

// Query executes a query that may return multiple rows.
// The caller must close the returned rows with rows.Close().
func Query(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) (pgx.Rows, error) {
	return pool.Query(ctx, sql, args...)
}

// Exec executes a query that returns no rows.
func Exec(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) (pgconn.CommandTag, error) {
	return pool.Exec(ctx, sql, args...)
}

// WithTransaction executes fn within a database transaction.
// If fn returns an error the transaction is rolled back; otherwise it is committed.
func WithTransaction(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db.WithTransaction: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db.WithTransaction: commit: %w", err)
	}

	return nil
}
