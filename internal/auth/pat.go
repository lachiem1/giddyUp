package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	defaultSecretService = "giddyup"
	defaultSecretUser    = "up_pat"
	defaultDBKeyUser     = "db_key"
)

var (
	keyringGet    = keyring.Get
	keyringSet    = keyring.Set
	keyringDelete = keyring.Delete
)

// LoadPAT loads the Up Personal Access Token.
//
// Order of precedence:
// 1) UP_PAT environment variable.
// 2) macOS Keychain item referenced by service/account.
func LoadPAT() (string, error) {
	if pat := strings.TrimSpace(os.Getenv("UP_PAT")); pat != "" {
		return pat, nil
	}

	pat, err := loadFromKeyring()
	if err != nil {
		return "", err
	}

	if pat == "" {
		return "", errors.New("up PAT is empty")
	}

	return pat, nil
}

// SavePAT stores the Up PAT in the system credential store.
func SavePAT(pat string) error {
	trimmed := strings.TrimSpace(pat)
	if trimmed == "" {
		return errors.New("up PAT cannot be empty")
	}

	service := envOrDefault("GIDDYUP_KEYCHAIN_SERVICE", defaultSecretService)
	account := envOrDefault("GIDDYUP_KEYCHAIN_ACCOUNT", defaultSecretUser)

	if err := keyringSet(service, account, trimmed); err != nil {
		return fmt.Errorf(
			"failed to store keyring item service=%q account=%q: %w",
			service,
			account,
			err,
		)
	}

	return nil
}

// RemovePAT deletes the stored Up PAT from the system credential store.
func RemovePAT() error {
	service := envOrDefault("GIDDYUP_KEYCHAIN_SERVICE", defaultSecretService)
	account := envOrDefault("GIDDYUP_KEYCHAIN_ACCOUNT", defaultSecretUser)

	if err := keyringDelete(service, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf(
			"failed to delete keyring item service=%q account=%q: %w",
			service,
			account,
			err,
		)
	}
	return nil
}

// HasStoredPAT reports whether a non-empty PAT exists in the system keychain.
func HasStoredPAT() (bool, error) {
	service := envOrDefault("GIDDYUP_KEYCHAIN_SERVICE", defaultSecretService)
	account := envOrDefault("GIDDYUP_KEYCHAIN_ACCOUNT", defaultSecretUser)

	secret, err := keyringGet(service, account)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf(
			"failed to read keyring item service=%q account=%q: %w",
			service,
			account,
			err,
		)
	}

	return strings.TrimSpace(secret) != "", nil
}

// LoadDBKey loads the locally stored database key from keyring.
func LoadDBKey() (string, error) {
	service := envOrDefault("GIDDYUP_KEYCHAIN_SERVICE", defaultSecretService)
	account := envOrDefault("GIDDYUP_DB_KEY_ACCOUNT", defaultDBKeyUser)

	secret, err := keyringGet(service, account)
	if err != nil {
		return "", fmt.Errorf(
			"failed to read keyring item service=%q account=%q: %w",
			service,
			account,
			err,
		)
	}

	return strings.TrimSpace(secret), nil
}

// SaveDBKey stores the database key in keyring.
func SaveDBKey(key string) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return errors.New("database key cannot be empty")
	}

	service := envOrDefault("GIDDYUP_KEYCHAIN_SERVICE", defaultSecretService)
	account := envOrDefault("GIDDYUP_DB_KEY_ACCOUNT", defaultDBKeyUser)

	if err := keyringSet(service, account, trimmed); err != nil {
		return fmt.Errorf(
			"failed to store keyring item service=%q account=%q: %w",
			service,
			account,
			err,
		)
	}

	return nil
}

func loadFromKeyring() (string, error) {
	service := envOrDefault("GIDDYUP_KEYCHAIN_SERVICE", defaultSecretService)
	account := envOrDefault("GIDDYUP_KEYCHAIN_ACCOUNT", defaultSecretUser)

	secret, err := keyringGet(service, account)
	if err != nil {
		return "", fmt.Errorf(
			"failed to read keyring item service=%q account=%q: %w",
			service,
			account,
			err,
		)
	}

	return strings.TrimSpace(secret), nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
