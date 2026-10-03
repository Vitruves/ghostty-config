BINARY_NAME=ghostty-config
BUILD_DIR=build

.PHONY: build clean install local-install test run export-collection shots

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/ghostty-config

clean:
	rm -rf $(BUILD_DIR)

install: build
	@echo "Installing to /usr/local/bin/ (requires sudo)..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Install to the user's own bin directory, no sudo required.
local-install: build
	@mkdir -p ~/.local/bin
	cp $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/
	@echo "Installed to ~/.local/bin/ — make sure it is on your PATH"

test:
	go test ./...

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# Regenerate themes/collection from the palettes compiled into the binary.
export-collection: build
	./$(BUILD_DIR)/$(BINARY_NAME) -export-collection themes/collection

# Draw the palette frames the tests produce and render them to PNG in
# build/shots, pictures included, to check alignment and legibility by eye.
shots:
	tools/shot.sh build/shots TestPaletteFrames
