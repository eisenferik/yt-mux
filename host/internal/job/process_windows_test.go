//go:build windows

package job

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestProcessCompletesInsideJob(t *testing.T) {
	process := startHelper(t, "yt-mux-job-exit")
	defer process.Close()
	if err := process.Wait(); err != nil {
		t.Fatalf("helper failed: %v", err)
	}
}

func TestTerminateStopsSuspendedAssignedProcess(t *testing.T) {
	process := startHelper(t, "yt-mux-job-wait")
	defer process.Close()
	if err := process.Terminate(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("terminated helper unexpectedly exited successfully")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("terminated helper did not exit promptly")
	}
}

func TestTerminateStopsDescendantProcess(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	process := startHelper(t, "yt-mux-job-tree", pidPath)
	defer process.Close()

	var descendantID uint64
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		payload, readErr := os.ReadFile(pidPath)
		if readErr == nil {
			descendantID, err = strconv.ParseUint(string(payload), 10, 32)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if descendantID == 0 {
		t.Fatal("helper did not create its descendant")
	}
	if err := process.Terminate(); err != nil {
		t.Fatal(err)
	}
	_ = process.Wait()

	descendant, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(descendantID))
	if err != nil {
		return
	}
	defer windows.CloseHandle(descendant)
	status, err := windows.WaitForSingleObject(descendant, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if status == uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("descendant survived Job Object termination")
	}
}

func startHelper(t *testing.T, args ...string) *Process {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, err := Start(executable, append([]string{"-test.run=TestJobHelperProcess", "--"}, args...))
	if err != nil {
		t.Fatal(err)
	}
	return process
}

func TestJobHelperProcess(t *testing.T) {
	if len(os.Args) == 0 {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "yt-mux-job-exit":
		return
	case "yt-mux-job-wait":
		time.Sleep(30 * time.Second)
	case "yt-mux-job-grandchild":
		time.Sleep(30 * time.Second)
	}
	if len(os.Args) >= 2 && os.Args[len(os.Args)-2] == "yt-mux-job-tree" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(executable, "-test.run=TestJobHelperProcess", "--", "yt-mux-job-grandchild")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		pidPath := os.Args[len(os.Args)-1]
		if err := os.WriteFile(pidPath+".tmp", []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			t.Fatal(err)
		}
		if err := os.Rename(pidPath+".tmp", pidPath); err != nil {
			_ = child.Process.Kill()
			t.Fatal(err)
		}
		_ = child.Wait()
	}
}
