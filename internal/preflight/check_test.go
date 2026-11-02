package preflight

import (
	"context"
	"github.com/Nanofaas/terraform-provider-multipass/internal/sshclient"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHostPreflight(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
for arg do command=$arg; done
case "$command" in
*"'uname' '-s'"*) echo "$CHECK_OS";;
*"'version'"*) printf '{"multipass":"1.17.0"}';;
*"'list'"*) if [ "$CHECK_AUTH" = no ]; then echo 'not authenticated' >&2; exit 1; fi; printf '{"list":[]}';;
*"'get' 'local.driver'"*) echo qemu;;
*"'/usr/bin/python3'"*) printf '[{"uid":0,"mode":16877},{"uid":0,"mode":16877},{"uid":0,"mode":16877},{"uid":0,"mode":16877},{"uid":0,"mode":33261}]';;
*"'--version'"*) printf '{"protocol_version":1}';;
*"'-ll'"*) echo 'Options: !authenticate';;
*"'check'"*) printf '%s' "$CHECK_HELPER";;
*) echo "unexpected command" >&2;exit 9;;
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
		name, os, auth, helper string
		networks, fail         bool
	}{
		{"VM only", "Linux", "yes", "", false, false},
		{"auth missing", "Linux", "no", "", false, true},
		{"wrong OS", "Darwin", "yes", "", false, true},
		{"helper missing", "Linux", "yes", "", true, true},
		{"protocol mismatch", "Linux", "yes", `{"protocol_version":2,"checks":[]}`, true, true},
		{"firewall warning", "Linux", "yes", `{"protocol_version":1,"checks":[{"name":"firewall","status":"WARN","detail":"FORWARD DROP"}]}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CHECK_OS", tc.os)
			t.Setenv("CHECK_AUTH", tc.auth)
			t.Setenv("CHECK_HELPER", tc.helper)
			checks := Run(context.Background(), ssh, "multipass", tc.networks)
			failed := false
			for _, c := range checks {
				failed = failed || c.Status == "FAIL"
			}
			if failed != tc.fail {
				t.Fatalf("checks=%+v", checks)
			}
		})
	}
}
