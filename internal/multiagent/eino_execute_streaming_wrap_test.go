package multiagent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/schema"
)

func TestEinoExecuteMixedBackgroundCommandReturnsAfterForegroundShell(t *testing.T) {
	w := &einoStreamingShellWrap{
		inner: &passthroughStreamingShell{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	sr, err := w.ExecuteStreaming(ctx, &filesystem.ExecuteRequest{
		Command: `sh -c 'printf "service-ready\n"; sleep 2' & sleep 0.1; echo foreground-done`,
	})
	if err != nil {
		t.Fatalf("ExecuteStreaming: %v", err)
	}
	defer sr.Close()

	var out strings.Builder
	for {
		resp, rerr := sr.Recv()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			t.Fatalf("Recv: %v", rerr)
		}
		if resp != nil {
			out.WriteString(resp.Output)
		}
	}
	elapsed := time.Since(start)
	if elapsed >= time.Second {
		t.Fatalf("mixed background command took %s; output=%q", elapsed, out.String())
	}
	if !strings.Contains(out.String(), "foreground-done") {
		t.Fatalf("output = %q, want foreground completion output", out.String())
	}
}

type passthroughStreamingShell struct{}

func (s *passthroughStreamingShell) ExecuteStreaming(ctx context.Context, input *filesystem.ExecuteRequest) (*schema.StreamReader[*filesystem.ExecuteResponse], error) {
	return executeStreamingShellUntilRootExit(ctx, input.Command)
}
