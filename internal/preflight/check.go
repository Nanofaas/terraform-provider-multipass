package preflight

import (
	"context"
	"fmt"
	"github.com/Nanofaas/terraform-provider-multipass/internal/hostnetwork"
	"github.com/Nanofaas/terraform-provider-multipass/internal/multipasscli"
	"github.com/Nanofaas/terraform-provider-multipass/internal/sshclient"
	"github.com/hashicorp/go-version"
	"strings"
)

// Run only reads host configuration. A warning does not assert connectivity.
func Run(ctx context.Context, ssh *sshclient.Client, binary string, networks bool) []hostnetwork.Check {
	var checks []hostnetwork.Check
	add := func(name, detail string, err error) {
		status := "PASS"
		if err != nil {
			status = "FAIL"
			detail = err.Error()
		}
		checks = append(checks, hostnetwork.Check{Name: name, Status: status, Detail: detail})
	}
	out, stderr, err := ssh.Run(ctx, nil, "uname", "-s")
	if err != nil {
		add("ssh", "", fmt.Errorf("verified Linux SSH connection failed: %w (%s)", err, stderr))
		return checks
	}
	if strings.TrimSpace(string(out)) != "Linux" {
		add("linux", "", fmt.Errorf("expected Linux, received %q", strings.TrimSpace(string(out))))
		return checks
	}
	add("ssh", "Verified SSH connection to Linux", nil)
	cli, err := multipasscli.NewClient(ctx, multipasscli.Config{SSH: ssh, BinaryPath: binary})
	if err != nil {
		add("multipass", "", err)
		return checks
	}
	ver, err := cli.Version(ctx)
	if err == nil {
		var v *version.Version
		v, err = version.NewVersion(ver)
		minimum := "1.13.0"
		if networks {
			minimum = "1.15.0"
		}
		if err == nil && v.LessThan(version.Must(version.NewVersion(minimum))) {
			err = fmt.Errorf("Multipass %s is older than required %s", ver, minimum)
		}
	}
	add("multipass", ver, err)
	_, err = cli.ListInstances(ctx, true)
	if err != nil {
		err = fmt.Errorf("Multipass access failed; authenticate this SSH user's client if needed: %w", err)
	}
	add("authentication", "Multipass list succeeded", err)
	if !networks {
		return checks
	}
	out, stderr, err = ssh.Run(ctx, nil, "env", "-u", "MULTIPASS_SERVER_ADDRESS", binary, "get", "local.driver")
	if err != nil {
		err = fmt.Errorf("cannot read driver: %w (%s)", err, stderr)
	} else if strings.TrimSpace(string(out)) != "qemu" {
		err = fmt.Errorf("managed networks require QEMU; received %q", strings.TrimSpace(string(out)))
	}
	add("driver", "qemu", err)
	report, err := hostnetwork.New(ssh).Check(ctx)
	add("network_helper", "Installed helper protocol 1 and noninteractive sudo access verified", err)
	if err == nil {
		checks = append(checks, report...)
	}
	return checks
}
