#!/usr/bin/env bash

curl -Lso go.tar.gz https://golang.org/dl/go1.17.linux-amd64.tar.gz
echo "6bf89fc4f5ad763871cf7eac80a2d594492de7a818303283f1366a7f6a30372d go.tar.gz" | sha256sum -c -
mkdir -p gobuild/golang
tar -C gobuild/golang -xzf go.tar.gz
mkdir -p gobuild/gopath
mkdir -p gobuild/gocache
export GOPATH="$PWD/gobuild/gopath"
export GOCACHE="$PWD/gobuild/gocache"
export GO_LANG="$PWD/gobuild/golang/go/bin"
export GO_COMPILED="$GOPATH/bin"
export PATH="$GO_LANG:$GO_COMPILED:$PATH"
go version
curl -Lso fyne-backspace-android.zip https://github.com/howeyc/fyne/archive/backspace-android.zip
unzip fyne-backspace-android.zip
pushd fyne-backspace-android
go build fyne.io/fyne/v2/cmd/fyne
popd
./fyne-backspace-android/fyne package -os android -release -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
if [[ $# -eq 0 ]]; then
	zip -d crocgui.apk "META-INF/*"
fi
