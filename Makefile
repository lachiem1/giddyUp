.PHONY: release-build dev-release

BIN_DIR := ./bin
BIN := $(BIN_DIR)/giddyup

release-build:
	@mkdir -p $(BIN_DIR)
	CC="$$(xcrun --find clang)" \
	CXX="$$(xcrun --find clang++)" \
	SDKROOT="$$(xcrun --sdk macosx --show-sdk-path)" \
	CGO_ENABLED=1 \
	go build -tags sqlcipher -o $(BIN) ./cmd/giddyup

dev-release: release-build
	@$(BIN)
