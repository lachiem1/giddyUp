//go:build integration
// +build integration

package upapi

import (
	"context"
	"testing"
)

func TestAccountsRoutesIntegration(t *testing.T) {
	client := integrationClient(t)

	accounts, err := client.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts() failed: %v", err)
	}
	if accounts == nil {
		t.Fatal("ListAccounts() returned nil response")
	}
	if len(accounts.Data) == 0 {
		t.Fatal("ListAccounts() returned no accounts")
	}

	accountID := accounts.Data[0].ID
	if accountID == "" {
		t.Fatal("first account id is empty")
	}

	account, err := client.GetAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccount() failed: %v", err)
	}
	if account.Data.ID != accountID {
		t.Fatalf("GetAccount() id mismatch: got %q want %q", account.Data.ID, accountID)
	}
}
