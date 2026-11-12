# List all network interfaces of a node
data "proxmox_network_interfaces" "all" {
  node_name = "pve"
}

# List the physical interfaces whose name starts with "nic" and that have link
data "proxmox_network_interfaces" "fabric_nics" {
  node_name = "pve"
  type      = "eth"

  filters = {
    name   = "^nic[0-9]+$"
    active = true
  }
}

output "fabric_nic_names" {
  value = data.proxmox_network_interfaces.fabric_nics.interfaces[*].name
}

# List every bridge a guest can attach to, including SDN VNets
data "proxmox_network_interfaces" "bridges" {
  node_name = "pve"
  type      = "any_bridge"
}
