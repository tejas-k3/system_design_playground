package main

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestRunRejectsBadFlags(t *testing.T) {
	if err := run(context.Background(), []string{"-no-such-flag"}, io.Discard); err == nil {
		t.Fatal("an unknown flag must be an error")
	}
	if err := run(context.Background(), []string{"-addr", " "}, io.Discard); err == nil {
		t.Fatal("an empty -addr must be an error")
	}
}

func TestRunServesAndShutsDownGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, []string{"-addr", "127.0.0.1:0"}, io.Discard) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after the context was cancelled")
	}
}
