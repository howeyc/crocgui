//go:build !windows

package main

import "net/url"

func netUse(u *url.URL, del bool) error { return nil }
