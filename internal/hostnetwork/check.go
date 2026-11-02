package hostnetwork

import (
	"context"
	"encoding/json"
	"fmt"
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
