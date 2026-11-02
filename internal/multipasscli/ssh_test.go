package multipasscli

import (
	"context"
	"errors"
	"github.com/todoroff/terraform-provider-multipass/internal/sshclient"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func testSSH(t *testing.T) *sshclient.Client {
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
