APP_NAME = PlexMusicDaemon
BUNDLE_NAME = $(APP_NAME).app
BUILD_DIR = build

ARC_SOURCES = main.m daemon.m ipc.m
NOARC_SOURCES = SPMediaKeyTap.m
FRAMEWORKS = -framework Cocoa -framework MediaPlayer -framework IOKit -framework Carbon
CFLAGS = -Wall

.PHONY: all clean install

all: $(BUILD_DIR)/$(BUNDLE_NAME)

$(BUILD_DIR)/$(BUNDLE_NAME): $(ARC_SOURCES) $(NOARC_SOURCES) Info.plist
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)/$(BUNDLE_NAME)/Contents/MacOS
	@mkdir -p $(BUILD_DIR)/$(BUNDLE_NAME)/Contents/Resources

	# Compile ARC sources
	clang $(CFLAGS) -fobjc-arc $(FRAMEWORKS) -c $(ARC_SOURCES)

	# Compile non-ARC sources
	clang $(CFLAGS) -fno-objc-arc $(FRAMEWORKS) -c $(NOARC_SOURCES)

	# Link all object files
	clang $(CFLAGS) $(FRAMEWORKS) -o $(BUILD_DIR)/$(BUNDLE_NAME)/Contents/MacOS/$(APP_NAME) *.o

	# Clean up object files
	rm -f *.o

	cp Info.plist $(BUILD_DIR)/$(BUNDLE_NAME)/Contents/Info.plist

	@echo "Build complete: $(BUILD_DIR)/$(BUNDLE_NAME)"

clean:
	rm -rf $(BUILD_DIR)

install: $(BUILD_DIR)/$(BUNDLE_NAME)
	@echo "Installing $(BUNDLE_NAME)..."
	@mkdir -p ~/Library/Application\ Support/PlexMusic
	cp -R $(BUILD_DIR)/$(BUNDLE_NAME) ~/Library/Application\ Support/PlexMusic/
	@echo "Installed to ~/Library/Application Support/PlexMusic/$(BUNDLE_NAME)"
	@echo ""
	@echo "To install LaunchAgent, run:"
	@echo "  make install-agent"

install-agent:
	@echo "Installing LaunchAgent..."
	cp com.plexmusic.daemon.plist ~/Library/LaunchAgents/
	launchctl load ~/Library/LaunchAgents/com.plexmusic.daemon.plist
	@echo "LaunchAgent installed and loaded"

uninstall-agent:
	@echo "Uninstalling LaunchAgent..."
	launchctl unload ~/Library/LaunchAgents/com.plexmusic.daemon.plist 2>/dev/null || true
	rm -f ~/Library/LaunchAgents/com.plexmusic.daemon.plist
	@echo "LaunchAgent uninstalled"

restart: install
	@echo "Restarting $(APP_NAME)..."
	@pkill -f $(APP_NAME) || true
	@open -a ~/Library/Application\ Support/PlexMusic/$(BUNDLE_NAME)
	@echo "$(APP_NAME) restarted"
