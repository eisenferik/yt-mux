package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

func explorerCommand(path string) *exec.Cmd {
	explorer := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	command := exec.Command(explorer)
	// Explorer parses /select itself and needs the path quoted after the comma.
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + explorer + `" /select,"` + path + `"`}
	return command
}

func updateCommand(installRoot string) *exec.Cmd {
	command := exec.Command(
		filepath.Join(os.Getenv("SystemRoot"), `System32\WindowsPowerShell\v1.0\powershell.exe`),
		"-NoProfile", "-ExecutionPolicy", "Bypass", "-NoExit",
		"-File", filepath.Join(installRoot, "update.ps1"), "-FromNotification",
	)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	return command
}

func startDetached(command *exec.Cmd) error {
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
