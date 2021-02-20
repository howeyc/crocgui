#!/usr/bin/env bash

curl -Lso go.tar.gz https://golang.org/dl/go1.16.linux-amd64.tar.gz
echo "013a489ebb3e24ef3d915abe5b94c3286c070dfe0818d5bca8108f1d6e8440d2 go.tar.gz" | sha256sum -c -
mkdir -p golang
tar -C golang -xzf go.tar.gz
mkdir -p gopath
export GOPATH="$PWD/gopath"
export GO_LANG="$PWD/golang/go/bin"
export GO_COMPILED="$PWD/bin"
export PATH="$GO_LANG:$GO_COMPILED:$PATH"
export ANDROID_SDK_ROOT=$$SDK$$
export ANDROID_NDK_ROOT=$$NDK$$
export PATH=$(pwd)/go/bin:$PATH
go version
./golang/go/bin/go get fyne.io/fyne/v2/cmd/fyne
./golang/go/bin/go get github.com/fyne-io/mobile\@v0.1.2
sed -i '38s/^EGLDisplay/extern EGLDisplay/' ./gopath/pkg/mod/github.com/fyne-io/mobile\@v0.1.2/app/android.go
sed -i '39s/^EGLSurface/extern EGLSurface/' ./gopath/pkg/mod/github.com/fyne-io/mobile\@v0.1.2/app/android.go
./gopath/bin/fyne package -os android -release -appID com.github.howeyc.crocgui -icon manifest/en-US/images/icon.png
 
