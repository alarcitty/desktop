//go:build !windows

package updater

import "errors"

// CanReplaceExecutable always returns false on unsupported platforms in this desktop updater.
func CanReplaceExecutable(_ string) bool {
	return false
}

// ApplyAndRelaunch is only supported on Windows builds.
func ApplyAndRelaunch(_, _ string, _ int, _ bool) error {
	return errors.New("automatic in-app replacement is only supported on Windows")
}
