#!/usr/bin/env bash

curl -Lso go.tar.gz https://golang.org/dl/go1.17.2.linux-amd64.tar.gz
echo "f242a9db6a0ad1846de7b6d94d507915d14062660616a61ef7c808a76e4f1676 go.tar.gz" | sha256sum -c -
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
go install fyne.io/fyne/v2/cmd/fyne\@v2.1.0
fyne version
if [[ $# -eq 0 ]]; then
	fyne package -os android -release -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
	zip -d crocgui.apk "META-INF/*"
else
	fyne package -os android -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
fi
