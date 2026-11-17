//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// CanReplaceExecutable reports whether the current process can write into its own executable directory.
func CanReplaceExecutable(exePath string) bool {
	if strings.TrimSpace(exePath) == "" {
		return false
	}
	dir := filepath.Dir(exePath)
	probe, err := os.CreateTemp(dir, ".mila-write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}

func escapePSString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

// ApplyAndRelaunch stages a hidden helper script that waits for the running process to exit,
// installs or swaps the binary, relaunches Mila, and deletes temporary artifacts.
func ApplyAndRelaunch(stagedPath, currentExePath string, currentPID int, isInstaller bool) error {
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("mila-apply-update-%d.ps1", currentPID))

	var body string
	if isInstaller {
		body = fmt.Sprintf(`$ErrorActionPreference = 'SilentlyContinue'
$targetPid = %d
$staged = '%s'
$targetExe = '%s'
$self = $MyInvocation.MyCommand.Path

$proc = Get-Process -Id $targetPid -ErrorAction SilentlyContinue
if ($proc) {
  $proc.WaitForExit(20000) | Out-Null
}
Start-Sleep -Milliseconds 250

Start-Process -FilePath $staged -ArgumentList '/S' -Verb RunAs -Wait
if (Test-Path -LiteralPath $targetExe) {
  Start-Process -FilePath $targetExe
}
Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $self -Force -ErrorAction SilentlyContinue
`, currentPID, escapePSString(stagedPath), escapePSString(currentExePath))
	} else {
		body = fmt.Sprintf(`$ErrorActionPreference = 'SilentlyContinue'
$targetPid = %d
$staged = '%s'
$targetExe = '%s'
$self = $MyInvocation.MyCommand.Path

$proc = Get-Process -Id $targetPid -ErrorAction SilentlyContinue
if ($proc) {
  $proc.WaitForExit(20000) | Out-Null
}

$moved = $false
for ($i = 0; $i -lt 20; $i++) {
  try {
    Copy-Item -LiteralPath $staged -Destination $targetExe -Force -ErrorAction Stop
    $moved = $true
    break
  } catch {
    Start-Sleep -Milliseconds 250
  }
}

if ($moved -and (Test-Path -LiteralPath $targetExe)) {
  Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
  Start-Process -FilePath $targetExe
}
Remove-Item -LiteralPath $self -Force -ErrorAction SilentlyContinue
`, currentPID, escapePSString(stagedPath), escapePSString(currentExePath))
	}

	if err := os.WriteFile(scriptPath, []byte(body), 0o600); err != nil {
		return err
	}

	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden",
		"-File", scriptPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return err
	}
	return nil
}
