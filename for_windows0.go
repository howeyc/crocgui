//go:build !windows

package main

import "net/url"

func useDav(u *url.URL, del bool) error { return nil }
