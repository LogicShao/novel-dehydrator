package db

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func testPool(t *testing.T) string {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	return url
}

func TestNew(t *testing.T) {
	url := testPool(t)
	ctx := context.Background()

	pool, err := New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping after New: %v", err)
	}
}

func TestInsertAndSelect(t *testing.T) {
	url := testPool(t)
	ctx := context.Background()

	pool, err := New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()

	// Create temp table
	_, err = Exec(ctx, pool, `CREATE TEMP TABLE test_users (id SERIAL PRIMARY KEY, name TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Insert
	tag, err := Exec(ctx, pool, `INSERT INTO test_users (name) VALUES ($1)`, "alice")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected 1 row affected, got %d", tag.RowsAffected())
	}

	// Select
	var name string
	row := QueryRow(ctx, pool, `SELECT name FROM test_users WHERE id = $1`, 1)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if name != "alice" {
		t.Fatalf("expected name 'alice', got %q", name)
	}
}

func TestQuery(t *testing.T) {
	url := testPool(t)
	ctx := context.Background()

	pool, err := New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()

	_, err = Exec(ctx, pool, `CREATE TEMP TABLE test_items (id SERIAL PRIMARY KEY, label TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	_, err = Exec(ctx, pool, `INSERT INTO test_items (label) VALUES ('a'), ('b'), ('c')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := Query(ctx, pool, `SELECT label FROM test_items ORDER BY id`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	var labels []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	if len(labels) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(labels))
	}
}

func TestWithTransactionCommit(t *testing.T) {
	url := testPool(t)
	ctx := context.Background()

	pool, err := New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()

	_, err = Exec(ctx, pool, `CREATE TEMP TABLE test_tx (id SERIAL PRIMARY KEY, val INT NOT NULL)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	err = WithTransaction(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO test_tx (val) VALUES ($1)`, 42)
		return err
	})
	if err != nil {
		t.Fatalf("WithTransaction: %v", err)
	}

	var val int
	row := QueryRow(ctx, pool, `SELECT val FROM test_tx WHERE id = $1`, 1)
	if err := row.Scan(&val); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if val != 42 {
		t.Fatalf("expected 42, got %d", val)
	}
}

func TestWithTransactionRollback(t *testing.T) {
	url := testPool(t)
	ctx := context.Background()

	pool, err := New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()

	_, err = Exec(ctx, pool, `CREATE TEMP TABLE test_tx2 (id SERIAL PRIMARY KEY, val INT NOT NULL)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	wantErr := errors.New("boom")
	err = WithTransaction(ctx, pool, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO test_tx2 (val) VALUES ($1)`, 99)
		if e != nil {
			return e
		}
		return wantErr
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify rollback: no rows should exist
	var count int
	row := QueryRow(ctx, pool, `SELECT count(*) FROM test_tx2`)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count scan: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", count)
	}
}

