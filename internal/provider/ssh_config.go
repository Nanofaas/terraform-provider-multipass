package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/todoroff/terraform-provider-multipass/internal/sshclient"
)

func readSSHConfig(c providerConfigModel, allowUnknown bool) (*sshclient.Config, error) {
	unknown := c.SSHHost.IsUnknown() || c.SSHUser.IsUnknown() || c.SSHPort.IsUnknown() || c.SSHIdentityFile.IsUnknown() || c.SSHKnownHostsFile.IsUnknown()
	if unknown {
		if allowUnknown {
			return nil, nil
		}
		return nil, fmt.Errorf("SSH configuration must be known before configuring the provider")
	}
	if c.SSHHost.IsNull() {
		if !c.SSHUser.IsNull() || !c.SSHPort.IsNull() || !c.SSHIdentityFile.IsNull() || !c.SSHKnownHostsFile.IsNull() {
			return nil, fmt.Errorf("SSH settings require ssh_host")
		}
		return nil, nil
	}
	if !c.ServerAddress.IsNull() {
		return nil, fmt.Errorf("ssh_host and server_address cannot be combined")
	}
	for _, v := range []types.String{c.SSHUser, c.SSHIdentityFile, c.SSHKnownHostsFile} {
		if !v.IsNull() && v.ValueString() == "" {
			return nil, fmt.Errorf("explicit SSH settings must not be empty")
		}
	}
	if !c.SSHPort.IsNull() && (c.SSHPort.ValueInt64() < 1 || c.SSHPort.ValueInt64() > 65535) {
		return nil, fmt.Errorf("ssh_port must be between 1 and 65535")
	}
	cfg := &sshclient.Config{Host: c.SSHHost.ValueString(), User: c.SSHUser.ValueString(), Port: int(c.SSHPort.ValueInt64()), IdentityFile: c.SSHIdentityFile.ValueString(), KnownHostsFile: c.SSHKnownHostsFile.ValueString()}
	return cfg, sshclient.Validate(*cfg)
}
func (p *MultipassProvider) ValidateConfig(ctx context.Context, req provider.ValidateConfigRequest, resp *provider.ValidateConfigResponse) {
	var c providerConfigModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &c)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := readSSHConfig(c, true); err != nil {
		resp.Diagnostics.AddError("Invalid SSH configuration", err.Error())
	}
}
