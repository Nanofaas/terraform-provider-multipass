package multipasscli

import (
	"bytes"
	"context"
	"errors"
	"github.com/Nanofaas/terraform-provider-multipass/internal/models"
	"github.com/Nanofaas/terraform-provider-multipass/internal/sshclient"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func testSSH(t *testing.T) *sshclient.Client {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX host fixture")
	}
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell required")
	}
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\nfor arg do last=$arg; done\nexec sh -c \"$last\"\n"), 0700)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ssh, err := sshclient.New(sshclient.Config{Host: "test-host"})
	if err != nil {
		t.Fatal(err)
	}
	return ssh
}
func TestSSHRemoteDaemonAndErrors(t *testing.T) {
	ssh := testSSH(t)
	t.Setenv("MULTIPASS_SERVER_ADDRESS", "wrong-host:50051")
	script := filepath.Join(t.TempDir(), "remote multipass")
	body := `#!/bin/sh
[ -z "$MULTIPASS_SERVER_ADDRESS" ] || exit 9
case "$1" in
version) printf '{"multipass":"1.16.4"}';;
missing) echo 'info failed: instance "missing" does not exist' >&2; exit 1;;
transport) echo 'host not found' >&2; exit 255;;
binary) echo 'multipass: command not found' >&2; exit 127;;
other) echo 'network not found' >&2; exit 1;;
wait) exec sleep 10;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	cli, err := NewClient(context.Background(), Config{BinaryPath: script, SSH: ssh})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cli.Version(context.Background()); err != nil || got != "1.16.4" {
		t.Fatalf("version=%q err=%v", got, err)
	}
	for _, command := range []string{"missing", "transport", "binary", "other"} {
		_, err := cli.(*client).run(context.Background(), command)
		if err == nil || errors.Is(err, ErrNotFound) != (command == "missing") {
			t.Errorf("%s: %v", command, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := cli.(*client).run(ctx, "wait"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout=%v", err)
	}
	// Remote paths must not be searched on the workstation.
	if _, err := NewClient(context.Background(), Config{BinaryPath: "remote-only-multipass", SSH: ssh}); err != nil {
		t.Fatal(err)
	}
}

func TestSSHCloudInitFile(t *testing.T) {
	ssh := testSSH(t)
	dir := t.TempDir()
	capture := filepath.Join(dir, "stdin")
	t.Setenv("SSH_CLOUD_CAPTURE", capture)
	script := filepath.Join(dir, "multipass")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" = info ]; then echo 'instance test does not exist' >&2; exit 1; fi\ncase \"$*\" in *'--cloud-init -'*) cat > \"$SSH_CLOUD_CAPTURE\";; *) exit 7;; esac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cli, err := NewClient(context.Background(), Config{SSH: ssh, BinaryPath: script})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "cloud ' config")
	input := []byte("#cloud-config\npackages: [curl]\n")
	if err := os.WriteFile(file, input, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cli.LaunchInstance(context.Background(), models.LaunchOptions{Name: "test", CloudInitFile: file}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(capture)
	if err != nil || !bytes.Equal(got, input) {
		t.Fatalf("content=%q err=%v", got, err)
	}
	os.Remove(capture)
	if err := cli.LaunchInstance(context.Background(), models.LaunchOptions{Name: "test", CloudInitFile: file + "missing"}); err == nil {
		t.Fatal("accepted missing file")
	}
	if _, err := os.Stat(capture); !os.IsNotExist(err) {
		t.Fatal("remote command ran for missing input")
	}
}
func TestSSHFileRoundTrip(t *testing.T) {
	ssh := testSSH(t)
	dir := t.TempDir()
	capture := filepath.Join(dir, "remote-bytes")
	t.Setenv("SSH_FILE_CAPTURE", capture)
	script := filepath.Join(dir, "multipass")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n[ \"$1\" = transfer ] || exit 8\nif [ \"$2\" = - ]; then cat > \"$SSH_FILE_CAPTURE\"; else cat \"$SSH_FILE_CAPTURE\"; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cli, err := NewClient(context.Background(), Config{SSH: ssh, BinaryPath: script})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{{}, {0, 255, 10, 13, 42}} {
		file := filepath.Join(dir, "local ' file")
		os.WriteFile(file, input, 0600)
		if err := cli.Transfer(context.Background(), TransferOptions{Sources: []string{file}, Destination: "vm:/file"}); err != nil {
			t.Fatal(err)
		}
		got, err := cli.TransferCapture(context.Background(), TransferOptions{Sources: []string{"vm:/file"}, Destination: "-"})
		if err != nil || !bytes.Equal(input, got) {
			t.Fatalf("round trip=%v err=%v", got, err)
		}
	}
}

func TestSSHMountUsesHostPath(t *testing.T) {
	ssh := testSSH(t)
	dir := t.TempDir()
	capture := filepath.Join(dir, "mount-args")
	t.Setenv("SSH_MOUNT_CAPTURE", capture)
	script := filepath.Join(dir, "multipass")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SSH_MOUNT_CAPTURE\"\n"), 0700)
	cli, err := NewClient(context.Background(), Config{SSH: ssh, BinaryPath: script})
	if err != nil {
		t.Fatal(err)
	}
	source := "/remote/only ' $(exit 8)"
	if err := cli.Mount(context.Background(), "vm", models.Mount{HostPath: source, InstancePath: "/mnt/data"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(capture)
	if err != nil || string(got) != "mount\n"+source+"\nvm:/mnt/data\n" {
		t.Fatalf("args=%q err=%v", got, err)
	}
}

// Stop at execution: a canceled context lets this test cover real workstation
// paths on Windows without needing a POSIX SSH fixture or a remote server.
func TestSSHUploadLocalPath(t *testing.T) {
	name := "file:with-colon"
	if runtime.GOOS == "windows" {
		name = "file.txt"
	}
	source := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(source, []byte{0, 255}, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cli := &client{ssh: &sshclient.Client{}, timeout: time.Second}
	err := cli.Transfer(ctx, TransferOptions{Sources: []string{source}, Destination: "vm:/file"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("local path rejected before execution: %v", err)
	}
}

func TestSSHLaunchUncertainAndPreexisting(t *testing.T) {
	ssh := testSSH(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "launched")
	t.Setenv("SSH_LAUNCH_MARKER", marker)
	script := filepath.Join(dir, "multipass")
	body := `#!/bin/sh
case "$1" in
info)
 if [ "$SSH_LAUNCH_MODE" = exists ]; then printf '{"info":{"test":{"state":"Running"}}}'; exit 0; fi
 echo 'info failed: instance "test" does not exist' >&2; exit 1;;
launch)
 touch "$SSH_LAUNCH_MARKER"
 if [ "$SSH_LAUNCH_MODE" = cancel ]; then exec sleep 10; fi
 echo 'Connection reset by peer' >&2; exit 255;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	cli, err := NewClient(context.Background(), Config{SSH: ssh, BinaryPath: script})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"disconnect", "cancel", "exists"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SSH_LAUNCH_MODE", mode)
			os.Remove(marker)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			if mode == "cancel" {
				go func() {
					defer close(done)
					deadline := time.Now().Add(3 * time.Second)
					for time.Now().Before(deadline) {
						if _, err := os.Stat(marker); err == nil {
							break
						}
						time.Sleep(5 * time.Millisecond)
					}
					cancel()
				}()
			} else {
				close(done)
			}
			err := cli.LaunchInstance(ctx, models.LaunchOptions{Name: "test"})
			<-done
			if errors.Is(err, ErrLaunchUncertain) != (mode != "exists") {
				t.Fatalf("unexpected launch result: %v", err)
			}
			if mode == "exists" {
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("existing VM was launched")
				}
			}
		})
	}
}
