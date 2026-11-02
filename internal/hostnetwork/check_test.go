package hostnetwork

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Nanofaas/terraform-provider-multipass/internal/sshclient"
)

func TestCheckRejectsUnsafeInstallationsAndMissingGrants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
for arg do command=$arg; done
case "$command" in
*"'/usr/bin/python3'"*) printf '%s' "$INSTALL_METADATA";;
*"'--version'"*) if [ "$ROOT_ONLY" = yes ]; then exit 1; fi; printf '{"protocol_version":1}';;
*"'-ll'"*) if [ "$MISSING_GRANT" = yes ]; then echo 'Options: authenticate'; else echo 'Options: !authenticate'; fi;;
*"'check'"*) printf '{"protocol_version":1,"checks":[{"name":"fixture","status":"PASS","detail":"ok"}]}';;
*) exit 9;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ssh, err := sshclient.New(sshclient.Config{Host: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, metadata, rootOnly, missing string
		fail                              bool
	}{
		{"secure", `[{"uid":0,"mode":16877},{"uid":0,"mode":33261}]`, "no", "no", false},
		{"user owned", `[{"uid":1000,"mode":16877},{"uid":0,"mode":33261}]`, "no", "no", true},
		{"writable helper", `[{"uid":0,"mode":16877},{"uid":0,"mode":33279}]`, "no", "no", true},
		{"writable ancestor", `[{"uid":0,"mode":16895},{"uid":0,"mode":33261}]`, "no", "no", true},
		{"symlink", `[{"uid":0,"mode":41471}]`, "no", "no", true},
		{"root only", `[{"uid":0,"mode":16877},{"uid":0,"mode":33216}]`, "yes", "no", true},
		{"password required", `[{"uid":0,"mode":16877},{"uid":0,"mode":33261}]`, "no", "yes", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("INSTALL_METADATA", strings.Replace(tc.metadata, "[", `[{"uid":0,"mode":16877},{"uid":0,"mode":16877},{"uid":0,"mode":16877},`, 1))
			t.Setenv("ROOT_ONLY", tc.rootOnly)
			t.Setenv("MISSING_GRANT", tc.missing)
			_, err := New(ssh).Check(context.Background())
			if (err != nil) != tc.fail {
				t.Fatalf("got error %v", err)
			}
		})
	}
}
