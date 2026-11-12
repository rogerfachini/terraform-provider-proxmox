# Jumbo frames on a physical interface that is used as a bridge port.
resource "proxmox_network_linux_eth" "nic0" {
  node_name = "pve"
  name      = "nic0"

  mtu     = 9000
  comment = "fabric uplink"
}

resource "proxmox_network_linux_bridge" "vmbr1" {
  node_name = "pve"
  name      = "vmbr1"

  ports = [proxmox_network_linux_eth.nic0.name]
  mtu   = 9000
}
