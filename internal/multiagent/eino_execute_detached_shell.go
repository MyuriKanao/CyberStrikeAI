package multiagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/schema"
)

func executeStreamingShellUntilRootExit(ctx context.Context, command string) (*schema.StreamReader[*filesystem.ExecuteResponse], error) {
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("command is required")
	}

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	sr, sw := schema.Pipe[*filesystem.ExecuteResponse](100)
	if err := cmd.Start(); err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
		go sendExecuteStreamError(sw, fmt.Errorf("failed to start command: %w", err))
		return sr, nil
	}
	_ = stdoutW.Close()
	_ = stderrW.Close()

	go streamShellUntilRootExit(ctx, cmd, stdoutR, stderrR, sw)
	return sr, nil
}

func streamShellUntilRootExit(ctx context.Context, cmd *exec.Cmd, stdout, stderr *os.File, sw *schema.StreamWriter[*filesystem.ExecuteResponse]) {
	var forwardOutput atomic.Bool
	forwardOutput.Store(true)
	defer func() {
		forwardOutput.Store(false)
		sw.Close()
	}()

	chunks := make(chan string, 128)
	startPipeDrainer(ctx, stdout, chunks, &forwardOutput)
	startPipeDrainer(ctx, stderr, chunks, &forwardOutput)

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	outputSent := false
	sendChunk := func(chunk string) bool {
		if chunk == "" {
			return false
		}
		outputSent = true
		return sw.Send(&filesystem.ExecuteResponse{Output: chunk}, nil)
	}

	var waitErr error
	waitDone := false
	for !waitDone {
		select {
		case chunk := <-chunks:
			if sendChunk(chunk) {
				return
			}
		case waitErr = <-waitCh:
			waitDone = true
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			waitErr = ctx.Err()
			waitDone = true
		}
	}

	quiet := time.NewTimer(200 * time.Millisecond)
	defer quiet.Stop()
	for {
		select {
		case chunk := <-chunks:
			if sendChunk(chunk) {
				return
			}
		case <-quiet.C:
			exitCode := 0
			if waitErr != nil {
				var exitErr *exec.ExitError
				if errors.As(waitErr, &exitErr) {
					exitCode = exitErr.ExitCode()
					_ = sw.Send(&filesystem.ExecuteResponse{
						Output:   fmt.Sprintf("command exited with non-zero code %d", exitCode),
						ExitCode: &exitCode,
					}, nil)
					return
				}
				_ = sw.Send(nil, fmt.Errorf("command failed: %w", waitErr))
				return
			}
			if !outputSent {
				_ = sw.Send(&filesystem.ExecuteResponse{ExitCode: &exitCode}, nil)
				return
			}
			_ = sw.Send(&filesystem.ExecuteResponse{ExitCode: &exitCode}, nil)
			return
		}
	}
}

func startPipeDrainer(ctx context.Context, r *os.File, chunks chan<- string, forwardOutput *atomic.Bool) {
	go func() {
		defer func() { _ = r.Close() }()
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				if forwardOutput == nil || !forwardOutput.Load() {
					_, _ = io.Copy(io.Discard, r)
					return
				}
				chunk := string(buf[:n])
				sent := false
				for forwardOutput.Load() && !sent {
					select {
					case chunks <- chunk:
						sent = true
					case <-ctx.Done():
						_, _ = io.Copy(io.Discard, r)
						return
					case <-time.After(50 * time.Millisecond):
					}
				}
				if !sent {
					_, _ = io.Copy(io.Discard, r)
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
}

func sendExecuteStreamError(sw *schema.StreamWriter[*filesystem.ExecuteResponse], err error) {
	defer sw.Close()
	_ = sw.Send(nil, err)
}
