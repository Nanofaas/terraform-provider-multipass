package provider

import (
	"github.com/todoroff/terraform-provider-multipass/internal/sshclient"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

type providerConfigModel struct {
	SSHHost           types.String `tfsdk:"ssh_host"`
	SSHUser           types.String `tfsdk:"ssh_user"`
	SSHPort           types.Int64  `tfsdk:"ssh_port"`
	SSHIdentityFile   types.String `tfsdk:"ssh_identity_file"`
	SSHKnownHostsFile types.String `tfsdk:"ssh_known_hosts_file"`
	MultipassPath     types.String `tfsdk:"multipass_path"`
	ServerAddress     types.String `tfsdk:"server_address"`
	CommandTimeout    types.Int64  `tfsdk:"command_timeout"`
	DefaultImage      types.String `tfsdk:"default_image"`
}

type providerConfig struct {
	BinaryPath     string
	ServerAddress  string
	CommandTimeout int
	DefaultImage   string
}

type providerData struct {
	ssh            *sshclient.Client
	client         multipasscli.Client
	defaultImage   string
	hostOS         string
	commandTimeout time.Duration
}
