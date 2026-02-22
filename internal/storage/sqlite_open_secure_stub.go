//go:build !sqlcipher
// +build !sqlcipher

package storage

import (
	"database/sql"
	"fmt"
)

func openSecureSQLite(path string, key string) (*sql.DB, error) {
	return nil, fmt.Errorf(
		"secure mode requires a sqlcipher-enabled build; rebuild with '-tags sqlcipher'",
	)
}

func secureSQLiteSupported() bool {
	return false
}
