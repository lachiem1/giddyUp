package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestLoadPATUsesEnvVarFirst(t *testing.T) {
	t.Setenv("UP_PAT", "  env-token  ")

	origGet := keyringGet
	defer func() { keyringGet = origGet }()

	keyringCalled := false
	keyringGet = func(service, user string) (string, error) {
		keyringCalled = true
		return "keyring-token", nil
	}

	got, err := LoadPAT()
	if err != nil {
		t.Fatalf("LoadPAT() unexpected error: %v", err)
	}
	if got != "env-token" {
		t.Fatalf("LoadPAT() = %q, want %q", got, "env-token")
	}
	if keyringCalled {
		t.Fatal("LoadPAT() called keyringGet even though UP_PAT was set")
	}
}

func TestLoadPATFallsBackToKeyring(t *testing.T) {
	t.Setenv("UP_PAT", "")
	t.Setenv("GIDDYUP_KEYCHAIN_SERVICE", "svc")
	t.Setenv("GIDDYUP_KEYCHAIN_ACCOUNT", "acct")

	origGet := keyringGet
	defer func() { keyringGet = origGet }()

	var gotService, gotUser string
	keyringGet = func(service, user string) (string, error) {
		gotService = service
		gotUser = user
		return "  keyring-token  ", nil
	}

	got, err := LoadPAT()
	if err != nil {
		t.Fatalf("LoadPAT() unexpected error: %v", err)
	}
	if got != "keyring-token" {
		t.Fatalf("LoadPAT() = %q, want %q", got, "keyring-token")
	}
	if gotService != "svc" || gotUser != "acct" {
		t.Fatalf("keyringGet called with (%q, %q), want (%q, %q)", gotService, gotUser, "svc", "acct")
	}
}

func TestLoadPATReturnsErrorWhenKeyringFails(t *testing.T) {
	t.Setenv("UP_PAT", "")

	origGet := keyringGet
	defer func() { keyringGet = origGet }()

	keyringGet = func(service, user string) (string, error) {
		return "", errors.New("boom")
	}

	_, err := LoadPAT()
	if err == nil {
		t.Fatal("LoadPAT() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "failed to read keyring item") {
		t.Fatalf("LoadPAT() error = %q, expected keyring read context", err.Error())
	}
}

func TestLoadPATReturnsErrorWhenTokenEmpty(t *testing.T) {
	t.Setenv("UP_PAT", "")

	origGet := keyringGet
	defer func() { keyringGet = origGet }()

	keyringGet = func(service, user string) (string, error) {
		return "   ", nil
	}

	_, err := LoadPAT()
	if err == nil {
		t.Fatal("LoadPAT() error = nil, want non-nil")
	}
	if err.Error() != "up PAT is empty" {
		t.Fatalf("LoadPAT() error = %q, want %q", err.Error(), "up PAT is empty")
	}
}

func TestSavePATSavesTrimmedToken(t *testing.T) {
	t.Setenv("GIDDYUP_KEYCHAIN_SERVICE", "svc")
	t.Setenv("GIDDYUP_KEYCHAIN_ACCOUNT", "acct")

	origSet := keyringSet
	defer func() { keyringSet = origSet }()

	var gotService, gotUser, gotSecret string
	keyringSet = func(service, user, secret string) error {
		gotService = service
		gotUser = user
		gotSecret = secret
		return nil
	}

	if err := SavePAT("  my-token  "); err != nil {
		t.Fatalf("SavePAT() unexpected error: %v", err)
	}
	if gotService != "svc" || gotUser != "acct" || gotSecret != "my-token" {
		t.Fatalf(
			"SavePAT() called keyringSet with (%q, %q, %q), want (%q, %q, %q)",
			gotService, gotUser, gotSecret, "svc", "acct", "my-token",
		)
	}
}

func TestSavePATRejectsEmptyToken(t *testing.T) {
	origSet := keyringSet
	defer func() { keyringSet = origSet }()

	called := false
	keyringSet = func(service, user, secret string) error {
		called = true
		return nil
	}

	err := SavePAT("   ")
	if err == nil {
		t.Fatal("SavePAT() error = nil, want non-nil")
	}
	if err.Error() != "up PAT cannot be empty" {
		t.Fatalf("SavePAT() error = %q, want %q", err.Error(), "up PAT cannot be empty")
	}
	if called {
		t.Fatal("SavePAT() called keyringSet for empty token")
	}
}

func TestSavePATReturnsErrorWhenKeyringSetFails(t *testing.T) {
	origSet := keyringSet
	defer func() { keyringSet = origSet }()

	keyringSet = func(service, user, secret string) error {
		return errors.New("write failed")
	}

	err := SavePAT("token")
	if err == nil {
		t.Fatal("SavePAT() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "failed to store keyring item") {
		t.Fatalf("SavePAT() error = %q, expected keyring write context", err.Error())
	}
}
