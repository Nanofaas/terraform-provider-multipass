package hostnetwork

import (
	"context"
	"github.com/Nanofaas/terraform-provider-multipass/internal/sshclient"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBridgeValidationAndConflicts(t *testing.T) {
	for _, tc := range []struct {
		name, address string
		valid         bool
	}{
		{"nf-test", "10.60.0.1/24", true}, {"nf-test", "192.168.20.1/24", true},
		{"this-name-is-too-long", "10.60.0.1/24", false}, {"bad;name", "10.60.0.1/24", false},
		{"nf-test", "8.8.8.8/24", false}, {"nf-test", "10.60.0.0/24", false}, {"nf-test", "10.60.0.255/24", false},
		{"nf-test", "10.60.0.1/32", false}, {"nf-test", "10.0.0.1/7", false}, {"nf-test", "fd00::1/64", false},
	} {
		if err := Validate(tc.name, tc.address); (err == nil) != tc.valid {
			t.Errorf("%v: %v", tc, err)
		}
	}
}
func TestBridgeHostHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX host fixture")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 required for helper tests")
	}
	cmd := exec.Command(python, "-m", "unittest", "discover", "-s", "scripts", "-p", "test_*.py", "-v")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestInvalidHelperResponse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX SSH fixture")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\nprintf '%s' \"$HELPER_RESPONSE\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ssh, err := sshclient.New(sshclient.Config{Host: "test-host"})
	if err != nil {
		t.Fatal(err)
	}
	c := New(ssh)
	for _, output := range []string{`{}`, `null`, `{"bridge":{"id":"wrong","name":"nf-test","address":"10.60.0.1/24"}}`} {
		t.Setenv("HELPER_RESPONSE", output)
		for _, action := range []string{"read", "create"} {
			requested := &Bridge{ID: "expected", Name: "nf-test", Address: "10.60.0.1/24"}
			got, err := c.call(context.Background(), action, requested)
			if err == nil {
				t.Errorf("%s %s accepted invalid response", action, output)
			}
			if action == "create" && (got == nil || got.ID != requested.ID) {
				t.Errorf("lost creation recovery UUID: %+v", got)
			}
		}
	}
}
