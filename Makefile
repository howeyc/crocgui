VSCODE_DIR := .vscode
SETTINGS_FILE := $(VSCODE_DIR)/settings.json
WSL_HOST_IP := $(shell ip route list default | awk '{print $$3}')

.PHONY: all clean arm arm64 386 amd64 linux window wsl darwin ios install darm emulator adb wsladb logcat atags tags wtags t windowsgui

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
	rm crocgui.apk

arm: 
	fyne package -os android/arm --release

arm64: 
	fyne package -os android/arm64 --release

386:
	fyne package -os android/386 --release

amd64:
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
