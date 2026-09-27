//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	apphost "github.com/eisenferik/yt-mux/host/internal/host"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != apphost.ExpectedOrigin {
		fmt.Fprintln(os.Stderr, "yt-mux: rejected native messaging caller origin")
		os.Exit(2)
	}
	executable, err := os.Executable()
	if err != nil {
		fail("could not resolve the host executable: %v", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		fail("could not resolve the host executable path: %v", err)
	}
	installRoot := filepath.Dir(filepath.Dir(executable))
	app := apphost.New(installRoot, os.Stdin, os.Stdout, os.Stderr)
	if err := app.Run(); err != nil {
		fail("native host stopped: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "yt-mux: "+format+"\n", args...)
	os.Exit(1)
}
