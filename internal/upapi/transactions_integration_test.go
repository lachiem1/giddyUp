//go:build integration
// +build integration

package upapi

import (
	"context"
	"testing"
)

func TestTransactionsRoutesIntegration(t *testing.T) {
	client := integrationClient(t)

	transactions, err := client.ListTransactions(context.Background(), TransactionListOptions{})
	if err != nil {
		t.Fatalf("ListTransactions() failed: %v", err)
	}
	if transactions == nil {
		t.Fatal("ListTransactions() returned nil response")
	}

	if len(transactions.Data) > 0 {
		transactionID := transactions.Data[0].ID
		transaction, err := client.GetTransaction(context.Background(), transactionID)
		if err != nil {
			t.Fatalf("GetTransaction() failed: %v", err)
		}
		if transaction.Data.ID != transactionID {
			t.Fatalf("GetTransaction() id mismatch: got %q want %q", transaction.Data.ID, transactionID)
		}
	}

	accounts, err := client.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts() failed: %v", err)
	}
	if len(accounts.Data) == 0 {
		t.Fatal("expected at least one account for ListTransactionsByAccount()")
	}

	_, err = client.ListTransactionsByAccount(
		context.Background(),
		accounts.Data[0].ID,
		TransactionListOptions{},
	)
	if err != nil {
		t.Fatalf("ListTransactionsByAccount() failed: %v", err)
	}
}
