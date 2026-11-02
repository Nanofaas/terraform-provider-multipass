package hostnetwork

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func (c *Client) Check(ctx context.Context) ([]Check, error) {
	if c.ssh == nil {
		return nil, fmt.Errorf("managed networks require SSH")
	}
	// Inspect the installation before trusting any code in the helper.
	metadata, stderr, err := c.ssh.Run(ctx, nil, "/usr/bin/python3", "-I", "-c", `import json,os,pathlib,sys
p=pathlib.Path(sys.argv[1])
print(json.dumps([{"uid":s.st_uid,"mode":s.st_mode} for s in (os.lstat(x) for x in [*reversed(p.parents),p])]))`, HelperPath)
	if err != nil {
		return nil, fmt.Errorf("inspect installed network helper: %w (%s)", err, stderr)
	}
	var files []struct{ UID, Mode uint32 }
	if err := json.Unmarshal(metadata, &files); err != nil || len(files) != 5 {
		return nil, fmt.Errorf("invalid helper installation metadata")
	}
	for i, file := range files {
		kind := uint32(0040000) // directory
		if i == len(files)-1 {
			kind = 0100000
		} // regular helper
		if file.UID != 0 || file.Mode&0022 != 0 || file.Mode&0170000 != kind {
			return nil, fmt.Errorf("helper and ancestors must be root-owned, non-writable by group/others, and not symbolic links")
		}
	}
	if _, stderr, err := c.ssh.Run(ctx, nil, HelperPath, "--version"); err != nil {
		return nil, fmt.Errorf("helper must be readable and executable by the SSH user: %w (%s)", err, stderr)
	}
	for _, action := range []string{"create", "delete"} {
		out, stderr, err := c.ssh.Run(ctx, nil, "env", "LC_ALL=C", "/usr/bin/sudo", "-n", "-ll", "--", HelperPath, action)
		nopasswd := false
		for _, line := range strings.Split(string(out), "\n") {
			if options, ok := strings.CutPrefix(strings.TrimSpace(line), "Options:"); ok {
				for _, option := range strings.Split(options, ",") {
					nopasswd = nopasswd || strings.TrimSpace(option) == "!authenticate"
				}
			}
		}
		if err != nil || !nopasswd {
			return nil, fmt.Errorf("helper %s requires an exact NOPASSWD sudoers grant (%s)", action, stderr)
		}
	}
	out, stderr, err := c.ssh.Run(ctx, []byte("{}"), "sudo", "-n", "--", HelperPath, "check")
	if err != nil {
		return nil, fmt.Errorf("install the network helper and restricted sudoers rules: %w (%s)", err, stderr)
	}
	var result struct {
		ProtocolVersion int     `json:"protocol_version"`
		Checks          []Check `json:"checks"`
		Error           string  `json:"error"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("invalid helper response: %w", err)
	}
	if result.ProtocolVersion != 1 {
		return nil, fmt.Errorf("helper protocol %d is incompatible; install protocol 1 helper", result.ProtocolVersion)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("helper check: %s", result.Error)
	}
	if len(result.Checks) == 0 {
		return nil, fmt.Errorf("helper returned no checks")
	}
	for _, check := range result.Checks {
		if check.Status != "PASS" && check.Status != "FAIL" && check.Status != "WARN" {
			return nil, fmt.Errorf("invalid helper check status")
		}
	}
	return result.Checks, nil
}
