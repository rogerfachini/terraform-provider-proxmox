//go:build acceptance || all

//testacc:tier=heavy
//testacc:resource=sdn

/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster/sdn"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster/sdn/fabrics"
	proxmoxtypes "github.com/bpg/terraform-provider-proxmox/proxmox/types"
)

// checkFabricNodeInterfaces asserts the interface property strings PVE holds for an OSPF fabric node,
// independent of Terraform state. PVE keeps keys in name, ip, network_type order.
func checkFabricNodeInterfaces(te *Environment, fabricID, nodeID string, want ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		client := (&cluster.Client{Client: te.Client()}).SDNFabricNodes(fabricID, fabrics.ProtocolOSPF)

		data, err := client.GetFabricNodeWithParams(
			context.Background(), nodeID, &sdn.QueryParams{Pending: proxmoxtypes.CustomBool(true).Pointer()},
		)
		if err != nil {
			return fmt.Errorf("reading fabric node %s/%s: %w", fabricID, nodeID, err)
		}

		got := slices.Sorted(slices.Values(data.Interfaces))
		slices.Sort(want)

		if !slices.Equal(got, want) {
			return fmt.Errorf("fabric node %s/%s interfaces: got %v, want %v", fabricID, nodeID, got, want)
		}

		return nil
	}
}

func TestAccResourceSDNFabricNodeOpenFabric(t *testing.T) {
	// Cannot run in parallel due to SDN applier functionality affecting global state

	te := InitEnvironment(t)

	tests := []struct {
		name  string
		steps []resource.TestStep
	}{
		{"create and update openfabric fabric node", []resource.TestStep{{
			Config: te.RenderConfig(`
				resource "proxmox_virtual_environment_sdn_fabric_openfabric" "fabric_openfabric" {
				  id         = "fabricS"
				  ip_prefix  = "10.0.0.0/16"
				  depends_on = [
				    proxmox_virtual_environment_sdn_applier.finalizer
				  ]
				}
				
				resource "proxmox_virtual_environment_sdn_fabric_node_openfabric" "fabric_node_openfabric" {
				  fabric_id  = proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric.id
				  node_id    = "pve"
				  ip         = "10.0.0.1"
				  interface_names = ["nic0"]
				  depends_on = [
					proxmox_virtual_environment_sdn_applier.finalizer
				  ]
				}

				resource "proxmox_virtual_environment_sdn_applier" "fabric_openfabric_applier" {
				  depends_on = [
				    proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric,
				    proxmox_virtual_environment_sdn_fabric_node_openfabric.fabric_node_openfabric,
				  ]
				}

				resource "proxmox_virtual_environment_sdn_applier" "finalizer" {}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_virtual_environment_sdn_fabric_node_openfabric.fabric_node_openfabric", map[string]string{
					"fabric_id":         "fabricS",
					"node_id":           "pve",
					"ip":                "10.0.0.1",
					"interface_names.#": "1",
					"interface_names.0": "nic0",
				}),
			),
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_virtual_environment_sdn_fabric_openfabric" "fabric_openfabric" {
				  id         = "fabricS"
				  ip_prefix  = "10.0.0.0/16"
				  depends_on = [
				    proxmox_virtual_environment_sdn_applier.finalizer
				  ]
				}

				resource "proxmox_virtual_environment_sdn_fabric_node_openfabric" "fabric_node_openfabric" {
				  fabric_id  = proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric.id
				  node_id    = "pve"
				  ip         = "10.0.0.2"
				  interface_names = []
				  depends_on = [
				    proxmox_virtual_environment_sdn_applier.finalizer
				  ]
				}

				resource "proxmox_virtual_environment_sdn_applier" "fabric_openfabric_applier" {
				  depends_on = [
				    proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric,
				    proxmox_virtual_environment_sdn_fabric_node_openfabric.fabric_node_openfabric,
				  ]
				}

				resource "proxmox_virtual_environment_sdn_applier" "finalizer" {}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_virtual_environment_sdn_fabric_node_openfabric.fabric_node_openfabric", map[string]string{
					"fabric_id":         "fabricS",
					"node_id":           "pve",
					"ip":                "10.0.0.2",
					"interface_names.#": "0",
				}),
				NoResourceAttributesSet("proxmox_virtual_environment_sdn_fabric_node_openfabric.fabric_node_openfabric", []string{
					"ip6",
				}),
			),
			ResourceName:      "proxmox_virtual_environment_sdn_fabric_node_openfabric.fabric_node_openfabric",
			ImportStateId:     "fabricS/pve",
			ImportState:       true,
			ImportStateVerify: true,
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: te.AccProviders,
				Steps:                    tt.steps,
			})
		})
	}
}

func TestAccResourceSDNFabricNodeOSPF(t *testing.T) {
	// Cannot run in parallel due to SDN applier functionality affecting global state

	te := InitEnvironment(t)

	tests := []struct {
		name  string
		steps []resource.TestStep
	}{
		{"create and update OSPF fabric", []resource.TestStep{{
			Config: te.RenderConfig(`
				resource "proxmox_virtual_environment_sdn_fabric_ospf" "fabric_ospf" {
				  id        = "fabricO"
				  area      = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_virtual_environment_sdn_fabric_node_ospf" "fabric_node_ospf" {
				  fabric_id = proxmox_virtual_environment_sdn_fabric_ospf.fabric_ospf.id
				  node_id   = "pve"
				  ip        = "10.0.0.1"
				  interface_names = ["nic0"]
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_virtual_environment_sdn_fabric_node_ospf.fabric_node_ospf", map[string]string{
					"fabric_id":         "fabricO",
					"node_id":           "pve",
					"ip":                "10.0.0.1",
					"interface_names.#": "1",
					"interface_names.0": "nic0",
				}),
			),
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_virtual_environment_sdn_fabric_ospf" "fabric_ospf" {
				  id    = "fabricO"
				  area  = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_virtual_environment_sdn_fabric_node_ospf" "fabric_node_ospf" {
				  fabric_id = proxmox_virtual_environment_sdn_fabric_ospf.fabric_ospf.id
				  node_id   = "pve"
				  ip      = "10.0.0.2"
				  interface_names = []
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_virtual_environment_sdn_fabric_node_ospf.fabric_node_ospf", map[string]string{
					"fabric_id":         "fabricO",
					"node_id":           "pve",
					"ip":                "10.0.0.2",
					"interface_names.#": "0",
				}),
			),
			ResourceName:      "proxmox_virtual_environment_sdn_fabric_node_ospf.fabric_node_ospf",
			ImportStateId:     "fabricO/pve",
			ImportState:       true,
			ImportStateVerify: true,
		}}},
		{"per-interface options", []resource.TestStep{{
			Config: te.RenderConfig(`
				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOI"
				  area      = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_sdn_fabric_node_ospf" "node" {
				  fabric_id = proxmox_sdn_fabric_ospf.fabric.id
				  node_id   = "pve"
				  ip        = "10.0.0.1"
				  interfaces = [
				    { name = "nic0", ip = "10.0.100.1/31", network_type = "point-to-point" },
				    { name = "nic1" },
				  ]
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_sdn_fabric_node_ospf.node", map[string]string{"interfaces.#": "2"}),
				resource.TestCheckTypeSetElemNestedAttrs("proxmox_sdn_fabric_node_ospf.node", "interfaces.*", map[string]string{
					"name": "nic0", "ip": "10.0.100.1/31", "network_type": "point-to-point",
				}),
				resource.TestCheckTypeSetElemNestedAttrs("proxmox_sdn_fabric_node_ospf.node", "interfaces.*", map[string]string{"name": "nic1"}),
				NoResourceAttributesSet("proxmox_sdn_fabric_node_ospf.node", []string{"interface_names.#"}),
				checkFabricNodeInterfaces(te, "fabricOI", "pve",
					"name=nic0,ip=10.0.100.1/31,network_type=point-to-point", "name=nic1"),
			),
		}, {
			// change one option, drop another, and move the options to the other interface
			Config: te.RenderConfig(`
				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOI"
				  area      = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_sdn_fabric_node_ospf" "node" {
				  fabric_id = proxmox_sdn_fabric_ospf.fabric.id
				  node_id   = "pve"
				  ip        = "10.0.0.1"
				  interfaces = [
				    { name = "nic0", network_type = "broadcast" },
				    { name = "nic1", ip = "10.0.100.3/31" },
				  ]
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckTypeSetElemNestedAttrs("proxmox_sdn_fabric_node_ospf.node", "interfaces.*", map[string]string{
					"name": "nic0", "network_type": "broadcast",
				}),
				resource.TestCheckTypeSetElemNestedAttrs("proxmox_sdn_fabric_node_ospf.node", "interfaces.*", map[string]string{
					"name": "nic1", "ip": "10.0.100.3/31",
				}),
				checkFabricNodeInterfaces(te, "fabricOI", "pve", "name=nic0,network_type=broadcast", "name=nic1,ip=10.0.100.3/31"),
			),
		}, {
			// options present: import must come back as interfaces
			ResourceName:      "proxmox_sdn_fabric_node_ospf.node",
			ImportStateId:     "fabricOI/pve",
			ImportState:       true,
			ImportStateVerify: true,
		}, {
			// switch to interface_names: the per-interface options must be gone from the node
			Config: te.RenderConfig(`
				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOI"
				  area      = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_sdn_fabric_node_ospf" "node" {
				  fabric_id = proxmox_sdn_fabric_ospf.fabric.id
				  node_id   = "pve"
				  ip        = "10.0.0.1"
				  interface_names = ["nic0"]
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_sdn_fabric_node_ospf.node", map[string]string{"interface_names.#": "1", "interface_names.0": "nic0"}),
				NoResourceAttributesSet("proxmox_sdn_fabric_node_ospf.node", []string{"interfaces.#"}),
				checkFabricNodeInterfaces(te, "fabricOI", "pve", "name=nic0"),
			),
		}, {
			// no options: import must come back as interface_names
			ResourceName:      "proxmox_sdn_fabric_node_ospf.node",
			ImportStateId:     "fabricOI/pve",
			ImportState:       true,
			ImportStateVerify: true,
		}, {
			// and back to interfaces without options
			Config: te.RenderConfig(`
				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOI"
				  area      = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_sdn_fabric_node_ospf" "node" {
				  fabric_id = proxmox_sdn_fabric_ospf.fabric.id
				  node_id   = "pve"
				  ip        = "10.0.0.1"
				  interfaces = [{ name = "nic1" }]
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_sdn_fabric_node_ospf.node", map[string]string{"interfaces.#": "1"}),
				NoResourceAttributesSet("proxmox_sdn_fabric_node_ospf.node", []string{"interface_names.#"}),
				checkFabricNodeInterfaces(te, "fabricOI", "pve", "name=nic1"),
			),
		}}},
		{"interface_names and interfaces are mutually exclusive", []resource.TestStep{{
			Config: te.RenderConfig(`
				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOX"
				  area      = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_sdn_fabric_node_ospf" "node" {
				  fabric_id = proxmox_sdn_fabric_ospf.fabric.id
				  node_id   = "pve"
				  ip        = "10.0.0.1"
				  interface_names = ["nic0"]
				  interfaces      = [{ name = "nic1" }]
				}
			`),
			ExpectError: regexp.MustCompile(`(?s)interface_names.*interfaces|Invalid Attribute Combination`),
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOX"
				  area      = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_sdn_fabric_node_ospf" "node" {
				  fabric_id = proxmox_sdn_fabric_ospf.fabric.id
				  node_id   = "pve"
				  ip        = "10.0.0.1"

				}
			`),
			ExpectError: regexp.MustCompile(`(?s)interface_names.*interfaces|Invalid Attribute Combination`),
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOX"
				  area      = "0"
				  ip_prefix = "10.0.0.0/16"
				}
				resource "proxmox_sdn_fabric_node_ospf" "node" {
				  fabric_id = proxmox_sdn_fabric_ospf.fabric.id
				  node_id   = "pve"
				  ip        = "10.0.0.1"
				  interfaces = [{ name = "nic0", ip = "2001:db8::1/64" }]
				}
			`),
			ExpectError: regexp.MustCompile(`must be a valid IPv4 address in CIDR notation`),
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: te.AccProviders,
				Steps:                    tt.steps,
			})
		})
	}
}
