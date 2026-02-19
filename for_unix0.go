//go:build !(unix && !android && !darwin)

package main

func registerScheme(scheme string) error { return nil }
