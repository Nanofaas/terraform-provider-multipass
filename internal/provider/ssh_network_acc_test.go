package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/todoroff/terraform-provider-multipass/internal/hostnetwork"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
	"github.com/todoroff/terraform-provider-multipass/internal/sshclient"
	"os"
	"strconv"
	"testing"
	"time"
)

func sshAcceptance(t *testing.T) (string, *sshclient.Client, multipasscli.Client) {
	t.Helper()
	if os.Getenv("TF_ACC") != "1" || os.Getenv("MULTIPASS_ACC_SSH_HOST") == "" {
		t.Skip("set TF_ACC=1 and MULTIPASS_ACC_SSH_HOST for authorized remote tests")
	}
	cfg := sshclient.Config{Host: os.Getenv("MULTIPASS_ACC_SSH_HOST"), User: os.Getenv("MULTIPASS_ACC_SSH_USER"), IdentityFile: os.Getenv("MULTIPASS_ACC_SSH_IDENTITY_FILE"), KnownHostsFile: os.Getenv("MULTIPASS_ACC_SSH_KNOWN_HOSTS_FILE")}
	if p := os.Getenv("MULTIPASS_ACC_SSH_PORT"); p != "" {
		var err error
		cfg.Port, err = strconv.Atoi(p)
		if err != nil {
			t.Fatal(err)
		}
	}
	ssh, err := sshclient.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := multipasscli.NewClient(context.Background(), multipasscli.Config{SSH: ssh})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := cli.ListInstances(ctx, true); err != nil {
		t.Fatalf("remote Multipass precheck: %v", err)
	}
	values := map[string]any{"ssh_host": cfg.Host}
	if cfg.User != "" {
		values["ssh_user"] = cfg.User
	}
	if cfg.Port != 0 {
		values["ssh_port"] = cfg.Port
	}
	if cfg.IdentityFile != "" {
		values["ssh_identity_file"] = cfg.IdentityFile
	}
	if cfg.KnownHostsFile != "" {
		values["ssh_known_hosts_file"] = cfg.KnownHostsFile
	}
	text := testProviderConfig + "\nprovider \"multipass\" {\n"
	for k, v := range values {
		b, _ := json.Marshal(v)
		text += fmt.Sprintf("%s = %s\n", k, b)
	}
	return text + "}\n", ssh, cli
}
func sshDestroyCheck(cli multipasscli.Client, nets *hostnetwork.Client) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, rs := range s.RootModule().Resources {
			if rs.Type == "multipass_network" {
				_, err := nets.Read(ctx, rs.Primary.ID)
				if !errors.Is(err, hostnetwork.ErrNotFound) {
					return fmt.Errorf("network cleanup: %v", err)
				}
			}
			if rs.Type == "multipass_instance" {
				_, err := cli.GetInstance(ctx, rs.Primary.ID)
				if !errors.Is(err, multipasscli.ErrNotFound) {
					return fmt.Errorf("VM cleanup: %v", err)
				}
			}
		}
		return nil
	}
}
func TestAccSSHPrivateNetwork(t *testing.T) {
	header, ssh, cli := sshAcceptance(t)
	subnet := os.Getenv("MULTIPASS_ACC_SUBNET")
	if subnet == "" {
		t.Fatal("set MULTIPASS_ACC_SUBNET to a dedicated non-overlapping private subnet")
	}
	name := randomName()
	suffix := name[len(name)-6:]
	conf := header + fmt.Sprintf(`
locals { subnet = %q }
resource "multipass_network" "lab" {
 name = %q
 address = "${cidrhost(local.subnet,1)}/${split("/",local.subnet)[1]}"
}
resource "multipass_instance" "node" {
 count = 2
 name = "%s-${count.index}"
 image = "24.04"
 cpus = 1
 memory = "1G"
 disk = "8G"
 networks {
  name = multipass_network.lab.name
  mode = "manual"
  mac = "52:54:%s:%s:%s:0${count.index}"
 }
 cloud_init = "#cloud-config\n${yamlencode({
  write_files = [{path="/etc/netplan/60-private.yaml",permissions="0600",content=yamlencode({network={version=2,ethernets={private0={match={macaddress="52:54:%s:%s:%s:0${count.index}"},dhcp4=false,addresses=["${cidrhost(local.subnet,11+count.index)}/${split("/",local.subnet)[1]}"]}}}})}]
  runcmd = [["netplan","apply"]]
 })}"
 wait_for_cloud_init = true
 timeouts { create = "15m" }
}
output "second_address" { value=cidrhost(local.subnet,12) }
`, subnet, name, name, suffix[:2], suffix[2:4], suffix[4:], suffix[:2], suffix[2:4], suffix[4:])
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: sshDestroyCheck(cli, hostnetwork.New(ssh)), Steps: []resource.TestStep{
		{Config: conf, Check: func(s *terraform.State) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return cli.Exec(ctx, name+"-0", []string{"ping", "-c", "2", "-W", "3", s.RootModule().Outputs["second_address"].Value.(string)})
		}},
		{Config: conf, PlanOnly: true, ExpectNonEmptyPlan: false},
		{ResourceName: "multipass_network.lab", ImportState: true, ImportStateVerify: true},
	}})
}
func TestAccSSHNetworkDrift(t *testing.T) {
	header, ssh, cli := sshAcceptance(t)
	name := randomName()
	subnet := os.Getenv("MULTIPASS_ACC_SUBNET")
	if subnet == "" {
		t.Fatal("set MULTIPASS_ACC_SUBNET")
	}
	conf := header + fmt.Sprintf(`
locals { subnet = %q }
resource "multipass_network" "lab" {
 name = %q
 address = "${cidrhost(local.subnet,1)}/${split("/",local.subnet)[1]}"
}
`, subnet, name)
	var id string
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: sshDestroyCheck(cli, hostnetwork.New(ssh)), Steps: []resource.TestStep{
		{Config: conf, Check: func(s *terraform.State) error {
			id = s.RootModule().Resources["multipass_network.lab"].Primary.ID
			return nil
		}},
		{PreConfig: func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err := hostnetwork.New(ssh).Read(ctx, id); err != nil {
				t.Fatal(err)
			}
			_, stderr, err := ssh.Run(ctx, nil, "sudo", "-n", "--", "nmcli", "connection", "down", "uuid", id)
			if err != nil {
				t.Fatalf("disable owned test bridge: %v %s", err, stderr)
			}
		}, Config: conf, Check: resource.TestCheckResourceAttr("multipass_network.lab", "active", "true")},
	}})
}
