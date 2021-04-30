#!/usr/bin/env bash

curl -Lso go.tar.gz https://golang.org/dl/go1.16.3.linux-amd64.tar.gz
echo "951a3c7c6ce4e56ad883f97d9db74d3d6d80d5fec77455c6ada6c1f7ac4776d2 go.tar.gz" | sha256sum -c -
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
go get fyne.io/fyne/v2/cmd/fyne\@v2.0.3-rc2
fyne version
fyne package -os android -release -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
zip -d crocgui.apk "META-INF/*"
