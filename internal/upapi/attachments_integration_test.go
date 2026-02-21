//go:build integration
// +build integration

package upapi

import (
	"context"
	"testing"
)

func TestAttachmentsRoutesIntegration(t *testing.T) {
	client := integrationClient(t)

	attachments, err := client.ListAttachments(context.Background())
	if err != nil {
		t.Fatalf("ListAttachments() failed: %v", err)
	}
	if attachments == nil {
		t.Fatal("ListAttachments() returned nil response")
	}

	if len(attachments.Data) == 0 {
		t.Skip("no attachments available in account, skipping GetAttachment route")
	}

	attachmentID := attachments.Data[0].ID
	attachment, err := client.GetAttachment(context.Background(), attachmentID)
	if err != nil {
		t.Fatalf("GetAttachment() failed: %v", err)
	}
	if attachment.Data.ID != attachmentID {
		t.Fatalf("GetAttachment() id mismatch: got %q want %q", attachment.Data.ID, attachmentID)
	}
}
