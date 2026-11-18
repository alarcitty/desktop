//go:build windows

package protocol

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Register creates the per-user mila:// URL protocol association (HKCU, no admin).
func Register(exePath string) error {
	exePath = strings.TrimSpace(exePath)
	if exePath == "" {
		return errors.New("protocol: empty executable path")
	}

	root, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\mila`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.SetStringValue("", "URL:Mila Protocol"); err != nil {
		return err
	}
	if err := root.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}

	cmd, _, err := registry.CreateKey(root, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer cmd.Close()
	return cmd.SetStringValue("", OpenCommand(exePath))
}