package sshclient

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeSSH(t *testing.T) *Client {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX host fixture")
	}
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell required")
	}
	c, err := New(Config{Host: "192.0.2.10", User: "ubuntu", Port: 2222, IdentityFile: "/keys/a b", KnownHostsFile: "/keys/known hosts"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	t.Setenv("SSH_TEST_CAPTURE", capture)
	script := filepath.Join(dir, "ssh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SSH_TEST_CAPTURE\"\nfor arg do last=$arg; done\nexec sh -c \"$last\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	c.binary = script
	return c
}
func TestSSHArgumentsAndStdin(t *testing.T) {
	c := fakeSSH(t)
	for _, input := range [][]byte{nil, {}, {0, 1, 255, 10}, []byte("#cloud-config\nhello")} {
		out, stderr, err := c.Run(context.Background(), input, "cat")
		if err != nil || len(stderr) != 0 || !bytes.Equal(out, input) {
			t.Fatalf("out=%q stderr=%q err=%v", out, stderr, err)
		}
	}
	literal := "a ' quote\n$(exit 7) `exit 8` ; *"
	out, _, err := c.Run(context.Background(), nil, "printf", "%s", literal)
	if err != nil || string(out) != literal {
		t.Fatalf("literal=%q err=%v", out, err)
	}
	args, err := os.ReadFile(os.Getenv("SSH_TEST_CAPTURE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"BatchMode=yes", "StrictHostKeyChecking=yes", "ForwardAgent=no", "-T", "2222", "/keys/a b", "192.0.2.10"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("missing %q: %s", want, args)
		}
	}
}
func TestSSHValidationAndCancellation(t *testing.T) {
	for _, cfg := range []Config{{}, {Host: "-oProxyCommand=bad"}, {Host: "a\nb"}, {Host: "user@host"}, {Host: "host", Port: 65536}, {Host: "host", User: "-root"}, {Host: "host", IdentityFile: "a\x00b"}} {
		if _, err := New(cfg); err == nil {
			t.Errorf("accepted invalid config %#v", cfg)
		}
	}
	for _, host := range []string{"192.0.2.10", "2001:db8::1", "my-host"} {
		if _, err := New(Config{Host: host}); err != nil {
			t.Fatal(err)
		}
	}
	c := fakeSSH(t)
	if _, _, err := c.Run(context.Background(), nil); err == nil {
		t.Fatal("accepted empty command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, _, err := c.Run(ctx, nil, "sleep", "10"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout=%v", err)
	}
	_, _, err := c.Run(context.Background(), nil, "sh", "-c", "exit 255")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 255 {
		t.Fatalf("exit=%v", err)
	}
}
