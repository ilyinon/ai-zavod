//go:build darwin || linux

package checks

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestCancellationStopsCheckProcessGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 30 & wait")
	configureCheckProcess(cmd)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled check succeeded")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("child process kept check alive after cancellation")
	}
}
