VSCODE_DIR := .vscode
SETTINGS_FILE := $(VSCODE_DIR)/settings.json
WSL_HOST_IP := $(shell ip route list default | awk '{print $$3}')
VERSION_NAME := $(shell grep -E '^\s*Version\s*=' FyneApp.toml | sed -E 's/^\s*Version\s*=\s*"([^"]+)".*/\1/')
BUILD_NUMBER := $(shell grep -E '^\s*Build\s*=' FyneApp.toml | sed -E 's/^\s*Build\s*=\s*([0-9]+).*/\1/')
DEB_FILE := crocgui_$(VERSION_NAME)_amd64.deb

.PHONY: all clean arm arm64 386 amd64 linux windows wsl darwin ios install darm emulator adb wsladb logcat atags tags wtags t windowsgui ver deb debi debr useri userr repo local relay

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

CROC_FORK := github.com/abakCroc/croc/v10
PEER_FORK := github.com/abakum/peerdiscovery
CROC_VERSION := $(shell go list -m -f '{{.Version}}' $(CROC_FORK)@latest 2>/dev/null)
PEER_VERSION := $(shell go list -m -f '{{.Version}}' $(PEER_FORK)@latest 2>/dev/null)

repo:
	@if [ -z "$(CROC_VERSION)" ]; then echo "ERROR: Cannot resolve $(CROC_FORK)@latest"; exit 1; fi
	@if [ -z "$(PEER_VERSION)" ]; then echo "ERROR: Cannot resolve $(PEER_FORK)@latest"; exit 1; fi
	@echo "Updating replace directives in go.mod:"
	@echo "  $(CROC_FORK) $(CROC_VERSION)"
	@echo "  $(PEER_FORK) $(PEER_VERSION)"
	@go mod edit -replace=github.com/schollz/croc/v10=$(CROC_FORK)@$(CROC_VERSION)
	@go mod edit -replace=github.com/schollz/peerdiscovery=$(PEER_FORK)@$(PEER_VERSION)
	@go mod tidy
	@echo "Done."

local:
	@echo "Switching replace directives to local paths in go.mod:"
	@go mod edit -replace=github.com/schollz/croc/v10=../croc
	@go mod edit -replace=github.com/schollz/peerdiscovery=../peerdiscovery
	@go mod tidy
	@echo "Done."

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

apk:
	apkanalyzer manifest print crocgui.apk

aapt:
	aapt2 dump badging crocgui.apk

apksigner:
	apksigner verify -v --print-certs crocgui.apk

align:
	$(ANDROID_HOME)/build-tools/35.0.0/zipalign -c -p -v 4 crocgui.apk
	
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

releasew: 
	CC=x86_64-w64-mingw32-gcc fyne release -os windows -certificate croc.p12 -profile "croc" -developer "CN=croc, OU=Personal, O=Konstantin Abakumov, L=Millerovo, ST=Rostov Oblast, C=RU" -password "$(CERT_PASS)"


signexe:
	#sudo apt-get update;sudo apt-get install osslsigncode
	osslsigncode sign -pkcs12 croc.p12 -pass "$(CERT_PASS)" \
		-n "croc" \
		-t http://timestamp.digicert.com \
		-in crocgui.exe -out crocgui-signed.exe

signps1:
	osslsigncode sign -pkcs12 croc.p12 -pass "$(CERT_PASS)" \
		-n "croc" \
		-in croc-unsigned.ps1 -out croc.ps1

cert:
	rm cert.exe; \
	osslsigncode sign \
		-pkcs12 croc.p12 \
		-pass "$(CERT_PASS)" \
		-n "croc" \
		-in cmd/cert/cert.exe \
		-out cert.exe

trust:
	GOOS=windows go build -ldflags="-s -w" -o tmp_build.exe ./cmd/I_trust_the_signer_of_this/
	rm ./cmd/I_trust_the_signer_of_this/I_trust_the_signer_of_this.exe || true
	osslsigncode sign \
		-pkcs12 croc.p12 \
		-pass "$(CERT_PASS)" \
		-n "croc" \
		-in tmp_build.exe \
		-out ./cmd/I_trust_the_signer_of_this/I_trust_the_signer_of_this.exe
	rm tmp_build.exe

links:
	adb shell pm get-app-links com.github.howeyc.crocgui

view:
	adb shell am start -a android.intent.action.VIEW -d "https://abakum.github.io/#123" com.github.howeyc.crocgui

signappx: 
	osslsigncode sign -pkcs12 croc.p12 -pass "$(CERT_PASS)" \
		-appx \
		-n "croc" \
		-t http://timestamp.digicert.com \
		-in crocgui.appx -out crocgui-signed.appx


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

relay:
	@CROC_SERVICE_BIN=$$(systemctl cat croc.service 2>/dev/null | grep -E '^ExecStart=' | head -1 | sed 's/ExecStart=//' | awk '{print $$1}'); \
	if [ -z "$$CROC_SERVICE_BIN" ]; then echo "ERROR: Cannot find ExecStart in croc.service"; exit 1; fi; \
	CROC_NEW=$$(which croc); \
	if [ -z "$$CROC_NEW" ]; then echo "ERROR: croc binary not found in PATH"; exit 1; fi; \
	echo "Service binary: $$CROC_SERVICE_BIN"; \
	echo "New binary:     $$CROC_NEW"; \
	sudo systemctl stop croc.service; \
	sudo cp "$$CROC_NEW" "$$CROC_SERVICE_BIN"; \
	sudo systemctl start croc.service; \
	sudo systemctl status croc.service

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

ialt:
	@echo "for Alt Linux..."
	sudo apt-get update
	sudo apt-get install -y \
		pkg-config \
		gcc \
		gcc-c++ \
		make \
		libGL-devel \
		libglfw-devel \
		libX11-devel \
		libXcursor-devel \
		libXrandr-devel \
		libXinerama-devel \
		libXi-devel \
		libXxf86vm-devel
	@echo "Alt Linux done"

ideb:
	@echo "for Debian/Ubuntu..."
	sudo apt-get update
	sudo apt-get install -y \
		pkg-config \
		gcc \
		g++ \
		make \
		libgl1-mesa-dev \
		libglfw3-dev \
		libgl-dev \
		libx11-dev \
		libxcursor-dev \
		libxrandr-dev \
		libxinerama-dev \
		libxi-dev
	@echo "Debian/Ubuntu done"