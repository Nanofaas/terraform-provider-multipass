package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProviderSSHConfiguration(t *testing.T) {
	for _, values := range []map[string]any{
		{"ssh_host": "server", "server_address": "other:50051"},
		{"ssh_user": "ubuntu"},
		{"ssh_host": tftypes.UnknownValue},
		{"ssh_host": "server", "ssh_port": int64(0)},
		{"ssh_host": "server", "ssh_identity_file": tftypes.UnknownValue},
	} {
		p := New("test")()
		cfg := providerTestConfig(t, p, values)
		var resp provider.ConfigureResponse
		p.Configure(context.Background(), provider.ConfigureRequest{Config: cfg}, &resp)
		if !resp.Diagnostics.HasError() || resp.ResourceData != nil {
			t.Errorf("accepted invalid config %v: %v", values, resp.Diagnostics)
		}
	}
}

func TestProviderSSHAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX host fixture")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell required")
	}
	dir := t.TempDir()
	body := `#!/bin/sh
for arg do host=$last; last=$arg; done
case "$last" in
*uname*) echo Linux;;
*version*) printf '{"multipass":"1.16.4"}';;
*list*) printf '{"list":[{"name":"%s","state":"Running"}]}' "$host";;
*) exit 7;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, host := range []string{"first-host", "second-host"} {
		p := New("test")()
		cfg := providerTestConfig(t, p, map[string]any{"ssh_host": host, "multipass_path": "remote-only-multipass"})
		var resp provider.ConfigureResponse
		p.Configure(context.Background(), provider.ConfigureRequest{Config: cfg}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		data := resp.ResourceData.(providerData)
		if data.ssh == nil || data.client != resp.DataSourceData.(providerData).client {
			t.Fatal("client not shared")
		}
		instances, err := data.client.ListInstances(context.Background(), false)
		if err != nil || len(instances) != 1 || instances[0].Name != host {
			t.Fatalf("%s: %v %v", host, instances, err)
		}
	}
}
