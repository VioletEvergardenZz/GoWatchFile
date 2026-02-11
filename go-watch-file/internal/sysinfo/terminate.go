package sysinfo

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

var (
	ErrInvalidPID                = errors.New("invalid pid")
	ErrProcessNotFound           = errors.New("process not found")
	ErrTerminatePermissionDenied = errors.New("permission denied")
)

type TerminateResult struct {
	PID     int32  `json:"pid"`
	Name    string `json:"name,omitempty"`
	Command string `json:"command,omitempty"`
	Signal  string `json:"signal"`
	Forced  bool   `json:"forced"`
}

func TerminateProcess(pid int32, force bool) (TerminateResult, error) {
	result := TerminateResult{
		PID:    pid,
		Signal: "TERM",
	}
	if pid <= 0 {
		return result, ErrInvalidPID
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return result, normalizeTerminateErr(err)
	}
	if name, nameErr := proc.Name(); nameErr == nil {
		result.Name = name
	}
	if command, cmdErr := proc.Cmdline(); cmdErr == nil {
		result.Command = command
	}

	running, err := proc.IsRunning()
	if err != nil {
		return result, normalizeTerminateErr(err)
	}
	if !running {
		return result, ErrProcessNotFound
	}

	if force {
		result.Signal = "KILL"
		result.Forced = true
		if err := proc.Kill(); err != nil {
			return result, normalizeTerminateErr(err)
		}
		return result, nil
	}

	if err := proc.Terminate(); err != nil {
		return result, normalizeTerminateErr(err)
	}
	exited, waitErr := waitProcessExit(proc, 2*time.Second)
	if waitErr != nil {
		return result, normalizeTerminateErr(waitErr)
	}
	if exited {
		return result, nil
	}

	result.Signal = "KILL"
	result.Forced = true
	if err := proc.Kill(); err != nil {
		return result, normalizeTerminateErr(err)
	}
	return result, nil
}

func waitProcessExit(proc *process.Process, timeout time.Duration) (bool, error) {
	if proc == nil {
		return false, fmt.Errorf("process is nil")
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		running, err := proc.IsRunning()
		if err != nil {
			if isProcessMissingErr(err) {
				return true, nil
			}
			return false, err
		}
		if !running {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(120 * time.Millisecond)
	}
}

func normalizeTerminateErr(err error) error {
	if err == nil {
		return nil
	}
	if isProcessMissingErr(err) {
		return ErrProcessNotFound
	}
	if isPermissionErr(err) {
		return ErrTerminatePermissionDenied
	}
	return err
}

func isProcessMissingErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "no such process") ||
		strings.Contains(msg, "process does not exist") ||
		strings.Contains(msg, "not found")
}

func isPermissionErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "access is denied")
}
