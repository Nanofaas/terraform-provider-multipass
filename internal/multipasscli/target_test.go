package multipasscli

import (
	"context"
	"strings"
	"testing"
)

func TestRejectUnsafeInstanceTargets(t *testing.T) {
	c := &client{binaryPath: "must-not-run"}
	for _, name := range []string{"--all", "", "vm.snapshot", "bad name", "a\nname"} {
		for _, operation := range []struct {
			name string
			call func() error
		}{
			{"delete", func() error { return c.DeleteInstance(context.Background(), name, true) }},
			{"start", func() error { return c.StartInstance(context.Background(), name) }},
			{"stop", func() error { return c.StopInstance(context.Background(), name, false) }},
			{"recover", func() error { return c.RecoverInstance(context.Background(), name) }},
			{"read", func() error { _, err := c.GetInstance(context.Background(), name); return err }},
		} {
			t.Run(operation.name+name, func(t *testing.T) {
				if err := operation.call(); err == nil || !strings.Contains(err.Error(), "invalid instance name") {
					t.Fatalf("unsafe target reached command runner: %v", err)
				}
			})
		}
	}
}
