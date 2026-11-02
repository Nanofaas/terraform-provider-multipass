// Package sshclient runs argument lists on a Linux host through verified OpenSSH.
package sshclient

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type Config struct {
	Host, User, IdentityFile, KnownHostsFile string
	Port                                     int
}
type Client struct {
	binary string
	config Config
}

var hostName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var userName = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]*[$]?$`)

func Validate(cfg Config) error {
	if !hostName.MatchString(cfg.Host) && net.ParseIP(cfg.Host) == nil {
		return fmt.Errorf("ssh_host must be an IP address or SSH host name, without user@ or options")
	}
	if cfg.User != "" && !userName.MatchString(cfg.User) {
		return fmt.Errorf("invalid ssh_user")
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("ssh_port must be between 1 and 65535 when specified")
	}
	for _, p := range []string{cfg.IdentityFile, cfg.KnownHostsFile} {
		if strings.ContainsFunc(p, unicode.IsControl) {
			return fmt.Errorf("SSH file paths must not contain control characters")
		}
	}
	return nil
}
func New(cfg Config) (*Client, error) {
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	binary, err := exec.LookPath("ssh")
	if err != nil {
		return nil, fmt.Errorf("OpenSSH is required: %w", err)
	}
	return &Client{binary: binary, config: cfg}, nil
}
func (c *Client) Run(ctx context.Context, stdin []byte, argv ...string) ([]byte, []byte, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, nil, fmt.Errorf("remote command is required")
	}
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		if strings.ContainsRune(arg, 0) {
			return nil, nil, fmt.Errorf("remote arguments cannot contain NUL")
		}
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "-o", "ConnectTimeout=15"}
	if c.config.User != "" {
		args = append(args, "-l", c.config.User)
	}
	if c.config.Port != 0 {
		args = append(args, "-p", fmt.Sprint(c.config.Port))
	}
	if c.config.IdentityFile != "" {
		args = append(args, "-i", c.config.IdentityFile)
	}
	if c.config.KnownHostsFile != "" {
		args = append(args, "-o", "UserKnownHostsFile="+quoteSSHPath(c.config.KnownHostsFile))
	}
	args = append(args, "--", c.config.Host, "exec "+strings.Join(quoted, " "))
	cmd := exec.CommandContext(ctx, c.binary, args...)
	cmd.WaitDelay = time.Second
	cmd.Stdin = bytes.NewReader(stdin)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out.Bytes(), stderr.Bytes(), err
}

// OpenSSH parses -o values using ssh_config quoting, separately from shell quoting.
func quoteSSHPath(p string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(p) + `"`
}
