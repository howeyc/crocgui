//go:build !(unix && !android)

package main

func registerScheme(scheme string) error { return nil }
