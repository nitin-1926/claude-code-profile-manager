BINARY_NAME=ccpm
BUILD_DIR=./bin
.PHONY: build install test clean dev

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
