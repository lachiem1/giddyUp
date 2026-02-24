package storage

import "strings"

func normalizeTransactionText(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	// Collapse any repeated whitespace (spaces/tabs/newlines) to a single space.
	return strings.Join(strings.Fields(trimmed), " ")
}

func normalizeTransactionMerchant(rawText, description string) string {
	raw := normalizeTransactionText(rawText)
	if raw != "" {
		return raw
	}
	return normalizeTransactionText(description)
}

func normalizeInternalTransferMerchant(
	accountName string,
	transferAccountName string,
	amountValueInBaseUnits int64,
	rawText string,
	description string,
) (string, bool) {
	account := normalizeTransactionText(accountName)
	transfer := normalizeTransactionText(transferAccountName)

	// Primary path: use relationship IDs + amount sign to determine flow direction.
	if account != "" && transfer != "" {
		switch {
		case amountValueInBaseUnits < 0:
			return "Internal: " + account + " -> " + transfer, true
		case amountValueInBaseUnits > 0:
			return "Internal: " + transfer + " -> " + account, true
		}
	}

	// Fallback path: infer from transfer text patterns.
	candidate := normalizeTransactionMerchant(rawText, description)
	lower := strings.ToLower(candidate)
	if strings.HasPrefix(lower, "transfer from ") && account != "" {
		source := strings.TrimSpace(candidate[len("Transfer from "):])
		source = normalizeTransactionText(source)
		if source != "" {
			return "Internal: " + source + " -> " + account, true
		}
	}
	if strings.HasPrefix(lower, "transfer to ") && account != "" {
		dest := strings.TrimSpace(candidate[len("Transfer to "):])
		dest = normalizeTransactionText(dest)
		if dest != "" {
			return "Internal: " + account + " -> " + dest, true
		}
	}

	return "", false
}
