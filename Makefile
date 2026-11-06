BINARY_NAME=ghostty-config
BUILD_DIR=build

.PHONY: build clean install local-install test run export-collection shots readme-shots site-shots

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

# Draw the screens the tests produce, in a dark terminal and in a light one,
# and render them to PNG in build/shots, pictures included, to check alignment
# and legibility by eye. Needs Pillow: PYTHON=path/to/python names an
# interpreter that has it.
shots:
	tools/shot.sh build/shots TestPaletteFrames

# Draw the README's pictures from the picture interface and frame them as
# windows. Needs Pillow, like shots.
readme-shots:
	rm -rf build/readme
	GHOSTTY_CONFIG_README=$(CURDIR)/build/readme go test ./internal/tui -run TestReadmeShots -count=1
	for n in themes window; do $${PYTHON:-python3} tools/frame.py build/readme/$$n.png img/$$n.png 1600; done

# Draw the website's pictures (docs/, served by GitHub Pages) and frame them
# as windows. Needs Pillow, like shots.
SITE_SHOTS=12b-chooser-new 12b-chooser-new-light 31-palette-themes 31-palette-themes-light 32-palette-setting 32-palette-setting-light 24-guided-2a 24-guided-2a-light 24-guided-3a 24-guided-3a-light
site-shots: readme-shots
	rm -rf build/pxshots
	GHOSTTY_CONFIG_PXSHOTS=$(CURDIR)/build/pxshots go test ./internal/tui -run TestPixelScreens -count=1
	@mkdir -p docs/img
	for n in themes themes-light window window-dark input input-light font-list font-list-light editor editor-light help help-light; do $${PYTHON:-python3} tools/frame.py build/readme/$$n.png docs/img/$$n.webp 1600; done
	for n in $(SITE_SHOTS); do $${PYTHON:-python3} tools/frame.py build/pxshots/$$n.png docs/img/$$n.webp 1600; done
	cp img/logo.png docs/img/logo.png
