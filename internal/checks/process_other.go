//go:build !darwin && !linux

package checks

import (
	"os/exec"
	"time"
)

func configureCheckProcess(cmd *exec.Cmd) { cmd.WaitDelay = 2 * time.Second }
