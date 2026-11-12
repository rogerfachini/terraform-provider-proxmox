resource "proxmox_sdn_prefix_list" "example" {
  id = "example"

  entries = [
    {
      seq    = 5
      action = "permit"
      prefix = "10.0.0.0/16"
      le     = 32
    },
    {
      seq    = 10
      action = "deny"
      prefix = "192.168.0.0/16"
      ge     = 24
      le     = 28
    },
  ]
}

# Prefix lists are staged in the pending SDN configuration until it is applied.
resource "proxmox_sdn_applier" "example" {
  lifecycle {
    replace_triggered_by = [
      proxmox_sdn_prefix_list.example,
    ]
  }

  depends_on = [
    proxmox_sdn_prefix_list.example,
  ]
}
