//go:build !windows

package protocol

// Register is a no-op on non-Windows platforms.
func Register(string) error { return nil }