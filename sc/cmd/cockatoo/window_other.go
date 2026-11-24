//go:build !windows

package main

// appPaths has nothing to read outside Windows.
func appPaths(exe string) []string { return nil }
