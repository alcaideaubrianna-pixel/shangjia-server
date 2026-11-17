package cmd

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/gogf/gf/v2/os/gcmd"
)

func TestStandaloneCommandWaitsForBusinessDrain(t *testing.T) {
	var completed atomic.Bool
	command := &gcmd.Command{Func: func(ctx context.Context, parser *gcmd.Parser) error {
		serverWg.Add(1)
		go func() {
			completed.Store(true)
			serverWg.Done()
		}()
		return nil
	}}
	wrapped := standaloneCommand(command)
	if err := wrapped.Func(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !completed.Load() {
		t.Fatal("standalone command returned before its business worker drained")
	}
}

func TestStandaloneHelpRetainsFrameworkRendering(t *testing.T) {
	command := &gcmd.Command{Name: "help", Description: "help-only command"}
	wrapped := standaloneCommand(command)
	if wrapped.Func != nil || wrapped.Name != command.Name || wrapped.Description != command.Description {
		t.Fatal("help-only command must retain its nil handler for framework rendering")
	}
}
