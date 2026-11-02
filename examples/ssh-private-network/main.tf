terraform {
  required_providers {
    multipass = {
      source = "registry.opentofu.org/nanofaas/multipass"
    }
  }
}

variable "ssh_host" { type = string }
variable "ssh_user" { type = string }
variable "ssh_port" {
  type    = number
  default = 22
}
variable "subnet" {
  type    = string
  default = "10.60.0.0/24"
}

provider "multipass" {
  ssh_host = var.ssh_host
  ssh_user = var.ssh_user
  ssh_port = var.ssh_port
}

resource "multipass_network" "lab" {
  name    = "nf-lab"
  address = "${cidrhost(var.subnet, 1)}/${split("/", var.subnet)[1]}"
}

locals {
  nodes = {
    first  = { host = 11, mac = "52:54:00:60:00:11" }
    second = { host = 12, mac = "52:54:00:60:00:12" }
  }
}

resource "multipass_instance" "node" {
  for_each = local.nodes
  name     = "nf-lab-${each.key}"
  image    = "24.04"
  cpus     = 1
  memory   = "1G"
  disk     = "8G"

  networks {
    name = multipass_network.lab.name
    mode = "manual"
    mac  = each.value.mac
  }

  cloud_init = "#cloud-config\n${yamlencode({
    write_files = [{
      path        = "/etc/netplan/60-nanofaas-private.yaml"
      permissions = "0600"
      content = yamlencode({
        network = {
          version = 2
          ethernets = {
            private0 = {
              match     = { macaddress = each.value.mac }
              dhcp4     = false
              addresses = ["${cidrhost(var.subnet, each.value.host)}/${split("/", var.subnet)[1]}"]
            }
          }
        }
      })
    }]
    runcmd = [["netplan", "apply"]]
  })}"
  wait_for_cloud_init = true

  timeouts { create = "15m" }
}

output "private_addresses" {
  value = { for name, node in local.nodes : name => cidrhost(var.subnet, node.host) }
}
output "reported_addresses" {
  value = { for name, node in multipass_instance.node : name => node.ipv4 }
}
