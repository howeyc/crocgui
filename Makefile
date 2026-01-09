VSCODE_DIR := .vscode
SETTINGS_FILE := $(VSCODE_DIR)/settings.json
WSL_HOST_IP := $(shell ip route list default | awk '{print $$3}')
VERSION_NAME := $(shell grep -E '^\s*Version\s*=' FyneApp.toml | sed -E 's/^\s*Version\s*=\s*"([^"]+)".*/\1/')
BUILD_NUMBER := $(shell grep -E '^\s*Build\s*=' FyneApp.toml | sed -E 's/^\s*Build\s*=\s*([0-9]+).*/\1/')
DEB_FILE := crocgui_$(VERSION_NAME)_amd64.deb

.PHONY: all clean arm arm64 386 amd64 linux windows wsl darwin ios install darm emulator adb wsladb logcat atags tags wtags t windowsgui ver deb debi debr useri userr

ver: AndroidManifest.xml
	@echo "Reading version and build number from FyneApp.toml..."
	@if [ -z "$(VERSION_NAME)" ] || [ -z "$(BUILD_NUMBER)" ]; then \
		echo "ERROR: Could not extract Version or Build number from FyneApp.toml."; \
		cat FyneApp.toml; \
		exit 1; \
	fi
	@echo "Extracted Version Name: $(VERSION_NAME)"
	@echo "Extracted Build Number: $(BUILD_NUMBER)"
	@echo "Updating AndroidManifest.xml with versionName=$(VERSION_NAME) and versionCode=$(BUILD_NUMBER)"
	@sed -i.bak "s/android:versionName=\"[^\"]*\"/android:versionName=\"$(VERSION_NAME)\"/g" AndroidManifest.xml
	@sed -i.bak "s/android:versionCode=\"[0-9]*\"/android:versionCode=\"$(BUILD_NUMBER)\"/g" AndroidManifest.xml
	@echo "Updated AndroidManifest.xml:"
	@grep -E 'android:versionName|android:versionCode' AndroidManifest.xml

atags:	
	@mkdir -p $(VSCODE_DIR)
	@if [ -f $(SETTINGS_FILE) ]; then \
		jq '.gopls["build.buildFlags"] = ["-tags=android"]' $(SETTINGS_FILE) > $(SETTINGS_FILE).tmp && \
		mv $(SETTINGS_FILE).tmp $(SETTINGS_FILE); \
	else \
		echo '{"gopls": {"build.buildFlags": ["-tags=android"]}}' > $(SETTINGS_FILE); \
	fi
	@echo "Enabling Android build tags for gopls press Ctrl+Shift+P Go: Restart Language Server"

wtags:	
	@mkdir -p $(VSCODE_DIR)
	@if [ -f $(SETTINGS_FILE) ]; then \
		jq '.gopls["build.buildFlags"] = ["-tags=android"]' $(SETTINGS_FILE) > $(SETTINGS_FILE).tmp && \
		mv $(SETTINGS_FILE).tmp $(SETTINGS_FILE); \
	else \
		echo '{"gopls": {"build.buildFlags": ["-tags=windows"]}}' > $(SETTINGS_FILE); \
	fi
	@echo "Enabling Windows build tags for gopls press Ctrl+Shift+P Go: Restart Language Server"

tags:
	@mkdir -p $(VSCODE_DIR)
	@if [ -f $(SETTINGS_FILE) ]; then \
		jq 'del(.gopls["build.buildFlags"])' $(SETTINGS_FILE) > $(SETTINGS_FILE).tmp && \
		mv $(SETTINGS_FILE).tmp $(SETTINGS_FILE); \
	else \
		echo '{}' > $(SETTINGS_FILE); \
	fi
	@echo "Reset build tags for gopls press Ctrl+Shift+P Go: Restart Language Server"

all: android

android: main.go send.go recv.go settings.go theme.go about.go AndroidManifest.xml fdroid-build.sh
	ANDROID_HOME=~/android bash fdroid-build.sh test

clean:
	go clean
	rm -f crocgui.apk crocgui.exe crocgui_*.deb crocgui*.xy
	rm -rf crocgui.app

arm: ver
	fyne package -os android/arm --release

arm64: ver
	fyne package -os android/arm64 --release

386: ver
	fyne package -os android/386 --release

amd64: ver
	fyne package -os android/amd64 --release

emulator:
	emulator -avd Medium_Phone_API_36.1

adb:
	adb install crocgui.apk

logcat:
	adb logcat|grep "croc    :"

wlogcat:
	cmd.exe /c C:\Users\KAbak\AppData\Local\Android\Sdk\platform-tools\adb logcat|find "croc    :"

wsladb:
	export ADB_SERVER_SOCKET=tcp:$(WSL_HOST_IP):5037

linux:
	fyne package -os linux --release

windows: 
	#sudo apt-get install gcc-mingw-w64-x86-64
	#go install fyne.io/tools/cmd/fyne@latest
	CC=x86_64-w64-mingw32-gcc fyne package -os windows --release

windowsgui:
	#GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -ldflags="-s -H windowsgui" -tags=opengl
	GOOS=windows CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 go build -ldflags="-s -H windowsgui"

mwindows:
	GOOS=windows CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 go build -ldflags="-s -extldflags=-mwindows"

wsl:
	GOOS=windows CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 go build -ldflags=-s

darwin: 
	fyne package -os darwin --release
	cp -r crocgui.app /Applications/

ios: 
	fyne package -os ios --release

install:
	GOFLAGS=-ldflags=-s go install

darm: 
	#brew install glfw
	GOARCH=arm64 fyne package -os darwin --release
	cp -r crocgui.app /Applications/

damd: 
	#brew install glfw
	GOARCH=amd64 fyne package -os darwin --release
	cp -r crocgui.app /Applications/

deb: crocgui.tar.xz build-deb.sh DEBIAN/control DEBIAN/postinst DEBIAN/prerm DEBIAN/postrm
	@echo "Building .deb package..."
	@chmod +x build-deb.sh
	@./build-deb.sh

debi: ver $(DEB_FILE)
	@echo "Installing $(DEB_FILE)..."
	@sudo dpkg -i "$(DEB_FILE)"

$(DEB_FILE):
	@if [ ! -f "$(DEB_FILE)" ]; then \
		echo "ERROR: $(DEB_FILE) not found. Run 'make deb' first."; \
		exit 1; \
	fi

debr: ver
	@echo "Removing crocgui package..."
	@sudo dpkg -r crocgui

debp: ver
	@echo "Purging crocgui package..."
	@sudo dpkg -P crocgui

useri: crocgui.tar.xz
	@echo "User installation from tar.xz..."
	@echo "Creating temporary directory..."
	@TEMP_DIR=$$(mktemp -d); \
	trap "rm -rf $$TEMP_DIR" EXIT; \
	echo "Extracting to $$TEMP_DIR..."; \
	tar -xf crocgui.tar.xz -C "$$TEMP_DIR"; \
	cd "$$TEMP_DIR"; \
	echo "Installing for current user..."; \
	make user-install; \
	echo "User installation completed! Installed to ~/.local/bin/"; \
	echo "Run it with: gtk-launch com.github.howeyc.crocgui"

userr: crocgui.tar.xz
	@echo "User uninstallation..."
	@echo "Creating temporary directory..."
	@TEMP_DIR=$$(mktemp -d); \
	trap "rm -rf $$TEMP_DIR" EXIT; \
	echo "Extracting to $$TEMP_DIR..."; \
	tar -xf crocgui.tar.xz -C "$$TEMP_DIR"; \
	cd "$$TEMP_DIR"; \
	echo "Uninstalling from user directory..."; \
	make user-uninstall; \
	echo "User uninstallation completed! Removed from ~/.local/"