package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lachiem1/giddyUp/internal/auth"
)

type Mode string

const (
	ModeSecure Mode = "secure"
)

type Config struct {
	Mode Mode
	Path string
}

func Open(ctx context.Context) (*sql.DB, Config, error) {
	cfg, err := configFromEnv()
	if err != nil {
		return nil, Config{}, err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
		return nil, Config{}, fmt.Errorf("create db directory: %w", err)
	}

	if !secureSQLiteSupported() {
		return nil, Config{}, fmt.Errorf(
			"secure mode requires a sqlcipher-enabled build; rebuild with '-tags sqlcipher'",
		)
	}

	key, created, err := ensureDBKey()
	if err != nil {
		return nil, Config{}, fmt.Errorf("ensure secure db key: %w", err)
	}
	if created {
		if err := resetLocalDBFiles(cfg.Path); err != nil {
			return nil, Config{}, fmt.Errorf("reset db after key creation: %w", err)
		}
	}

	db, err := openSecureSQLite(cfg.Path, key)
	if err != nil {
		return nil, Config{}, err
	}
	if err := runMigrations(ctx, db); err != nil {
		db.Close()
		return nil, Config{}, err
	}

	return db, cfg, nil
}

// Wipe removes local database files for the resolved DB path.
func Wipe() (Config, error) {
	cfg, err := configFromEnv()
	if err != nil {
		return Config{}, err
	}
	if err := resetLocalDBFiles(cfg.Path); err != nil {
		return Config{}, fmt.Errorf("wipe local db files: %w", err)
	}
	return cfg, nil
}

func configFromEnv() (Config, error) {
	if dbPath := strings.TrimSpace(os.Getenv("GIDDYUP_DB_PATH")); dbPath != "" {
		return Config{
			Mode: ModeSecure,
			Path: dbPath,
		}, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return Config{}, fmt.Errorf("resolve executable path: %w", err)
	}
	resolvedExePath, err := filepath.EvalSymlinks(exePath)
	if err == nil {
		exePath = resolvedExePath
	}
	exeDir := filepath.Dir(exePath)

	return Config{
		Mode: ModeSecure,
		Path: filepath.Join(exeDir, "giddyup.db"),
	}, nil
}

func ensureDBKey() (key string, created bool, err error) {
	key, err = auth.LoadDBKey()
	if err == nil && strings.TrimSpace(key) != "" {
		return key, false, nil
	}

	newKey, err := generateRandomKey()
	if err != nil {
		return "", false, err
	}

	if err := auth.SaveDBKey(newKey); err != nil {
		return "", false, err
	}
	return newKey, true, nil
}

func generateRandomKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(buf), nil
}

func runMigrations(ctx context.Context, db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  version INTEGER NOT NULL
);

INSERT OR IGNORE INTO schema_migrations (id, version) VALUES (1, 1);
`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("run sqlite migrations: %w", err)
	}
	return nil
}

func resetLocalDBFiles(path string) error {
	paths := []string{
		path,
		path + "-wal",
		path + "-shm",
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
