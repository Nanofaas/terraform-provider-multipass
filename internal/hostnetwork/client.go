package hostnetwork

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"

	"github.com/Nanofaas/terraform-provider-multipass/internal/sshclient"
)

const HelperPath = "/usr/local/libexec/nanofaas-multipass-network"

var ErrNotFound = errors.New("network not found")
var bridgeName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,14}$`)

type Bridge struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Active  bool   `json:"active"`
}
type Client struct{ ssh *sshclient.Client }

func New(ssh *sshclient.Client) *Client { return &Client{ssh: ssh} }
func Validate(name, address string) error {
	if !bridgeName.MatchString(name) {
		return fmt.Errorf("bridge name must start with a letter and contain at most 15 letters, digits, underscores or hyphens")
	}
	p, err := netip.ParsePrefix(address)
	if err != nil || !p.Addr().Is4() || !p.Addr().IsPrivate() || p.Bits() < 8 || p.Bits() > 30 {
		return fmt.Errorf("address must be a private IPv4 host/prefix (/8–/30)")
	}
	if !p.Masked().Addr().IsPrivate() {
		return fmt.Errorf("subnet must remain within private address space")
	}
	b := p.Addr().As4()
	n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	mask := uint32(1)<<(32-p.Bits()) - 1
	if n&mask == 0 || n&mask == mask {
		return fmt.Errorf("network and broadcast addresses cannot be assigned to the bridge")
	}
	// Both ends must stay within one RFC1918 block.
	end := n | mask
	last := netip.AddrFrom4([4]byte{byte(end >> 24), byte(end >> 16), byte(end >> 8), byte(end)})
	if !last.IsPrivate() {
		return fmt.Errorf("subnet must remain within private address space")
	}
	return nil
}
func (c *Client) Create(ctx context.Context, name, address string) (*Bridge, error) {
	if err := Validate(name, address); err != nil {
		return nil, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	id[6] = (id[6] & 15) | 64
	id[8] = (id[8] & 63) | 128
	b := &Bridge{ID: fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), Name: name, Address: address}
	return c.call(ctx, "create", b)
}
func (c *Client) Read(ctx context.Context, id string) (*Bridge, error) {
	return c.call(ctx, "read", &Bridge{ID: id})
}
func (c *Client) Delete(ctx context.Context, id string) error {
	_, err := c.call(ctx, "delete", &Bridge{ID: id})
	return err
}
func (c *Client) call(ctx context.Context, action string, b *Bridge) (*Bridge, error) {
	if c.ssh == nil {
		return nil, fmt.Errorf("managed networks require ssh_host")
	}
	input, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	args := []string{HelperPath, action}
	if action != "read" {
		args = append([]string{"sudo", "-n", "--"}, args...)
	}
	out, stderr, err := c.ssh.Run(ctx, input, args...)
	if err != nil {
		if action == "create" {
			return b, fmt.Errorf("network creation outcome unknown; refresh this UUID before retrying: %w (%s)", err, stderr)
		}
		return nil, fmt.Errorf("host network command failed (install the root-owned network helper and its restricted sudoers rules; NetworkManager and iproute2 are required): %w (%s)", err, stderr)
	}
	var result struct {
		ProtocolVersion int     `json:"protocol_version"`
		Bridge          *Bridge `json:"bridge"`
		Error           string  `json:"error"`
		Missing         bool    `json:"missing"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return b, fmt.Errorf("invalid network helper response: %w", err)
	}
	if result.ProtocolVersion != 1 {
		return b, fmt.Errorf("incompatible network helper protocol %d; install the helper shipped with this provider (protocol 1)", result.ProtocolVersion)
	}
	if result.Bridge != nil && (result.Bridge.ID != b.ID || !bridgeName.MatchString(result.Bridge.Name)) {
		return b, fmt.Errorf("invalid network helper response: bridge identity mismatch")
	}
	if result.Error != "" {
		return result.Bridge, errors.New(result.Error)
	}
	if result.Missing {
		return nil, ErrNotFound
	}
	if action != "delete" && result.Bridge == nil {
		return b, fmt.Errorf("invalid network helper response: bridge is missing")
	}
	return result.Bridge, nil
}
