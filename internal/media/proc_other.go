//go:build !windows

package media

import "os/exec"

func configureHiddenProcess(_ *exec.Cmd) {}
