BINARY  := todo-tui
VERSION := 0.1.0

# Default: build for current platform
.PHONY: build
build:
	go build -ldflags="-s -w" -o $(BINARY) .

# Cross-compile for major platforms
.PHONY: release
release: clean
	GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY)-darwin-arm64 .
	GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-arm64 .

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -rf dist/

.PHONY: install
install: build
	cp $(BINARY) $(HOME)/.local/bin/$(BINARY)

.PHONY: uninstall
uninstall:
	rm -f $(HOME)/.local/bin/$(BINARY)
