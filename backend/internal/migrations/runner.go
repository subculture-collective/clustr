// Package migrations applies Clustr's historical baseline and ordered SQL changes.
package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	baselineFile   = "schema.sql"
	baselineCutoff = "000027_graph_versioning.up.sql"
	advisoryLockID = int64(0x434c55535452) // "CLUSTR"
)

const CurrentMigration = "000036_telemetry_user_activity_index.up.sql"

// VerifyCurrent fails closed when a service is launched without the documented
// migration runner having applied the schema it was compiled against.
func VerifyCurrent(ctx context.Context, database *sql.DB) error {
	var applied bool
	err := database.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM clustr_schema_migrations WHERE filename=$1
	)`, CurrentMigration).Scan(&applied)
	if err != nil {
		return fmt.Errorf("read schema migration state: %w", err)
	}
	if !applied {
		return fmt.Errorf("schema is incompatible: required migration %s is not applied", CurrentMigration)
	}
	return nil
}

// Runner is the single schema migration entry point used by deployed services.
// Historical databases are validated and baselined at migration 27; fresh
// databases receive schema.sql before later immutable migrations are applied.
type Runner struct {
	DB  *sql.DB
	Dir string
}

// Run applies all pending *.up.sql files in deterministic filename order.
func (r Runner) Run(ctx context.Context) error {
	if r.DB == nil {
		return errors.New("migration database is required")
	}
	if strings.TrimSpace(r.Dir) == "" {
		return errors.New("migration directory is required")
	}

	if _, err := r.DB.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() { _, _ = r.DB.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockID) }()

	files, err := migrationFiles(r.Dir)
	if err != nil {
		return err
	}
	if err := r.ensureHistory(ctx); err != nil {
		return err
	}
	if err := r.ensureBaseline(ctx, files); err != nil {
		return err
	}

	for _, name := range files {
		if err := r.apply(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func migrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func (r Runner) ensureHistory(ctx context.Context) error {
	_, err := r.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS clustr_schema_migrations (
			filename TEXT PRIMARY KEY,
			checksum_sha256 TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create migration history: %w", err)
	}
	return nil
}

func (r Runner) ensureBaseline(ctx context.Context, files []string) error {
	var coreExists bool
	if err := r.DB.QueryRowContext(ctx, `SELECT to_regclass('public.subreddits') IS NOT NULL`).Scan(&coreExists); err != nil {
		return fmt.Errorf("inspect baseline: %w", err)
	}
	if !coreExists {
		body, err := os.ReadFile(filepath.Join(r.Dir, baselineFile))
		if err != nil {
			return fmt.Errorf("read baseline: %w", err)
		}
		tx, err := r.DB.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin baseline: %w", err)
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply baseline: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit baseline: %w", err)
		}
	}

	if err := r.validateLegacyBaseline(ctx); err != nil {
		return err
	}
	for _, name := range files {
		if name > baselineCutoff {
			break
		}
		if err := r.recordBaseline(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func (r Runner) validateLegacyBaseline(ctx context.Context) error {
	var ok bool
	err := r.DB.QueryRowContext(ctx, `
		SELECT to_regclass('public.crawl_jobs') IS NOT NULL
		   AND to_regclass('public.graph_nodes') IS NOT NULL
		   AND to_regclass('public.graph_versions') IS NOT NULL
		   AND EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'graph_nodes' AND column_name = 'pos_z'
		   )`).Scan(&ok)
	if err != nil {
		return fmt.Errorf("validate legacy baseline: %w", err)
	}
	if !ok {
		return errors.New("database schema is older than the supported migration-27 baseline; restore or migrate it with the legacy procedure before startup")
	}
	return nil
}

func (r Runner) recordBaseline(ctx context.Context, name string) error {
	sum, err := fileChecksum(filepath.Join(r.Dir, name))
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, `
		INSERT INTO clustr_schema_migrations(filename, checksum_sha256)
		VALUES ($1, $2)
		ON CONFLICT (filename) DO NOTHING`, name, sum)
	if err != nil {
		return fmt.Errorf("record baseline %s: %w", name, err)
	}
	return r.verifyChecksum(ctx, name, sum)
}

func (r Runner) apply(ctx context.Context, name string) error {
	path := filepath.Join(r.Dir, name)
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	sum := checksum(body)

	var existing string
	err = r.DB.QueryRowContext(ctx, `SELECT checksum_sha256 FROM clustr_schema_migrations WHERE filename = $1`, name).Scan(&existing)
	if err == nil {
		if existing != sum {
			return fmt.Errorf("migration %s checksum drift: database=%s filesystem=%s", name, existing, sum)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read migration history for %s: %w", name, err)
	}
	if requiresNoTransaction(body) {
		if _, err = r.DB.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply non-transactional migration %s: %w", name, err)
		}
		if _, err = r.DB.ExecContext(ctx, `INSERT INTO clustr_schema_migrations(filename, checksum_sha256) VALUES ($1, $2)`, name, sum); err != nil {
			return fmt.Errorf("record non-transactional migration %s: %w", name, err)
		}
		return nil
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	if _, err = tx.ExecContext(ctx, string(body)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO clustr_schema_migrations(filename, checksum_sha256) VALUES ($1, $2)`, name, sum); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}

func requiresNoTransaction(body []byte) bool {
	return strings.Contains(strings.ToUpper(string(body)), "CREATE INDEX CONCURRENTLY")
}

func (r Runner) verifyChecksum(ctx context.Context, name, expected string) error {
	var actual string
	if err := r.DB.QueryRowContext(ctx, `SELECT checksum_sha256 FROM clustr_schema_migrations WHERE filename = $1`, name).Scan(&actual); err != nil {
		return fmt.Errorf("verify migration %s: %w", name, err)
	}
	if actual != expected {
		return fmt.Errorf("migration %s checksum drift: database=%s filesystem=%s", name, actual, expected)
	}
	return nil
}

func fileChecksum(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read migration checksum %s: %w", filepath.Base(path), err)
	}
	return checksum(body), nil
}

func checksum(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
