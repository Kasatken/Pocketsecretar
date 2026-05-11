package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed *.sql
var files embed.FS

func Run(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return err
	}

	entries, err := files.ReadDir(".")
	if err != nil {
		return err
	}

	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		content, readErr := files.ReadFile(name)
		if readErr != nil {
			return readErr
		}

		tx, beginErr := db.BeginTx(ctx, nil)
		if beginErr != nil {
			return beginErr
		}
		if _, execErr := tx.ExecContext(ctx, string(content)); execErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s failed: %w", name, execErr)
		}
		if _, execErr := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); execErr != nil {
			_ = tx.Rollback()
			return execErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return commitErr
		}
	}

	return nil
}
