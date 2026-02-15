//go:build !unix

package main

func registerScheme(scheme string) error { return nil }
