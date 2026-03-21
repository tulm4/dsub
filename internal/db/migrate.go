package db

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationRunner executes SQL schema migrations.
// Based on: docs/service-decomposition.md §3.3
// Migration files follow the naming convention: 001_description.up.sql / .down.sql
type MigrationRunner struct {
	pool *pgxpool.Pool
}

// Migration represents a single schema migration.
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// NewMigrationRunner creates a new migration runner.
func NewMigrationRunner(pool *pgxpool.Pool) *MigrationRunner {
	return &MigrationRunner{pool: pool}
}

// EnsureMigrationTable creates the migration tracking table if it doesn't exist.
func (m *MigrationRunner) EnsureMigrationTable(ctx context.Context) error {
	sql := `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INT PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`
	_, err := m.pool.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("db: create migration table: %w", err)
	}
	return nil
}

// GetAppliedVersions returns the list of applied migration versions.
func (m *MigrationRunner) GetAppliedVersions(ctx context.Context) ([]int, error) {
	rows, err := m.pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("db: query applied versions: %w", err)
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("db: scan version: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// Apply runs a single migration and records it in the tracking table.
func (m *MigrationRunner) Apply(ctx context.Context, migration Migration) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin migration tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, migration.SQL); err != nil {
		return fmt.Errorf("db: execute migration %d (%s): %w", migration.Version, migration.Description, err)
	}

	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version, description, applied_at) VALUES ($1, $2, $3)",
		migration.Version, migration.Description, time.Now().UTC()); err != nil {
		return fmt.Errorf("db: record migration %d: %w", migration.Version, err)
	}

	return tx.Commit(ctx)
}

// ParseMigrations reads .up.sql migration files from an fs.FS and returns sorted Migrations.
// Files must follow naming: NNN_description.up.sql where NNN is zero-padded version.
func ParseMigrations(filesystem fs.FS) ([]Migration, error) {
	var migrations []Migration

	err := fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
			return nil
		}

		// Parse version from filename: "001_description.up.sql"
		name := d.Name()
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			return fmt.Errorf("db: invalid migration filename: %s", name)
		}

		var version int
		if _, err := fmt.Sscanf(parts[0], "%d", &version); err != nil {
			return fmt.Errorf("db: invalid migration version in %s: %w", name, err)
		}

		description := strings.TrimSuffix(parts[1], ".up.sql")

		content, err := fs.ReadFile(filesystem, path)
		if err != nil {
			return fmt.Errorf("db: read migration %s: %w", name, err)
		}

		migrations = append(migrations, Migration{
			Version:     version,
			Description: description,
			SQL:         string(content),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}
