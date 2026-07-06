BINARY_NAME=ccpm
BUILD_DIR=./bin
.PHONY: build install test clean dev desktop desktop-dev

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME) 2>/dev/null || \
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

test:
	go test -v ./...

dev: build
	go install .

clean:
	rm -rf $(BUILD_DIR)

# --- CCPM Desktop (Wails GUI) ---
# Requires the Wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@latest
# Builds a native app into desktop/build/bin (CCPM.app on macOS). Unsigned (v1).
desktop:
	cd desktop && wails build

# Hot-reload dev window for the desktop app.
desktop-dev:
	cd desktop && wails dev
