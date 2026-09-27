//go:build windows

package job

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Process struct {
	cmd       *exec.Cmd
	job       windows.Handle
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	terminate sync.Once
	close     sync.Once
}

func Start(executable string, args []string) (*Process, error) {
	jobHandle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}
	cleanupJob := true
	defer func() {
		if cleanupJob {
			windows.CloseHandle(jobHandle)
		}
	}()

	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		jobHandle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		return nil, fmt.Errorf("configure job object: %w", err)
	}

	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Suspension closes the small race in which a child could otherwise
		// create descendants before it is assigned to the Job Object.
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
		HideWindow:    true,
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create child stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create child stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start child process: %w", err)
	}

	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("open child process: %w", err)
	}
	assignErr := windows.AssignProcessToJobObject(jobHandle, processHandle)
	windows.CloseHandle(processHandle)
	if assignErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("assign child process to job object: %w", assignErr)
	}
	if err := resumeProcess(uint32(cmd.Process.Pid)); err != nil {
		_ = windows.TerminateJobObject(jobHandle, 1)
		_ = cmd.Wait()
		return nil, fmt.Errorf("resume child process: %w", err)
	}

	cleanupJob = false
	return &Process{cmd: cmd, job: jobHandle, stdout: stdout, stderr: stderr}, nil
}

func resumeProcess(processID uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return err
	}
	for {
		if entry.OwnerProcessID == processID {
			thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if err != nil {
				return err
			}
			_, resumeErr := windows.ResumeThread(thread)
			windows.CloseHandle(thread)
			return resumeErr
		}
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return fmt.Errorf("no thread found for process %d", processID)
			}
			return err
		}
	}
}

func (p *Process) Stdout() io.Reader { return p.stdout }
func (p *Process) Stderr() io.Reader { return p.stderr }

func (p *Process) Wait() error {
	return p.cmd.Wait()
}

func (p *Process) Terminate() error {
	var err error
	p.terminate.Do(func() {
		err = windows.TerminateJobObject(p.job, 1)
	})
	return err
}

func (p *Process) Close() error {
	var err error
	p.close.Do(func() {
		err = windows.CloseHandle(p.job)
	})
	return err
}
