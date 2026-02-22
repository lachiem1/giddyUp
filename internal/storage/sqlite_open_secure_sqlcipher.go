//go:build sqlcipher
// +build sqlcipher

package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

func openSecureSQLite(path string, key string) (*sql.DB, error) {
	escapedPath := url.PathEscape(path)
	escapedKey := url.QueryEscape(key)
	dsn := fmt.Sprintf(
		"file:%s?_pragma_key=%s&_pragma_cipher_page_size=4096&_pragma_kdf_iter=256000",
		escapedPath,
		escapedKey,
	)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlcipher db: %w", err)
	}

	if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		db.Close()
		return nil, fmt.Errorf("set db permissions: %w", err)
	}

	return db, nil
}

func secureSQLiteSupported() bool {
	return true
}
