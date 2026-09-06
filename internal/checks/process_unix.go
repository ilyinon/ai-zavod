//go:build darwin || linux

package checks

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// A cancelled go/pytest launcher must not leave its test processes running.
func configureCheckProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
