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
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster/sdn"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster/sdn/fabrics"
	proxmoxtypes "github.com/bpg/terraform-provider-proxmox/proxmox/types"
)

// checkFabricRouteFilter asserts the route filter PVE holds for the fabric, independent of Terraform state.
func checkFabricRouteFilter(te *Environment, protocol, id, want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		client := (&cluster.Client{Client: te.Client()}).SDNFabrics(protocol)

		data, err := client.GetFabricWithParams(
			context.Background(), id, &sdn.QueryParams{Pending: proxmoxtypes.CustomBool(true).Pointer()},
		)
		if err != nil {
			return fmt.Errorf("reading fabric %q: %w", id, err)
		}

		got := ""
		if data.RouteFilter != nil {
			got = *data.RouteFilter
		}

		if got != want {
			return fmt.Errorf("fabric %q route_filter: got %q, want %q", id, got, want)
		}

		return nil
	}
}

func TestAccResourceSDNFabricOpenFabric(t *testing.T) {
	// Cannot run in parallel due to SDN applier functionality affecting global state

	te := InitEnvironment(t)

	tests := []struct {
		name  string
		steps []resource.TestStep
	}{
		{"create and update fabrics", []resource.TestStep{{
			Config: te.RenderConfig(`
				resource "proxmox_virtual_environment_sdn_fabric_openfabric" "fabric_openfabric" {
				  id         = "fabricS"
				  ip_prefix  = "10.0.0.0/16"
				  depends_on = [
				    proxmox_virtual_environment_sdn_applier.finalizer
				  ]
				}

				resource "proxmox_virtual_environment_sdn_applier" "fabric_openfabric_applier" {
				  depends_on = [
				    proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric
				  ]
				}

				resource "proxmox_virtual_environment_sdn_applier" "finalizer" {}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric", map[string]string{
					"id":        "fabricS",
					"ip_prefix": "10.0.0.0/16",
				}),
			),
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_virtual_environment_sdn_fabric_openfabric" "fabric_openfabric" {
				  id  = "fabricS"
				  ip_prefix = "10.0.1.0/16"
				  depends_on = [
				    proxmox_virtual_environment_sdn_applier.finalizer
				  ]
				}

				resource "proxmox_virtual_environment_sdn_applier" "fabric_openfabric_applier" {
				  depends_on = [
				    proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric
				  ]
				}

				resource "proxmox_virtual_environment_sdn_applier" "finalizer" {}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric", map[string]string{
					"id":        "fabricS",
					"ip_prefix": "10.0.1.0/16",
				}),
				NoResourceAttributesSet("proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric", []string{
					"ip6_prefix",
				}),
			),
			ResourceName:      "proxmox_virtual_environment_sdn_fabric_openfabric.fabric_openfabric",
			ImportStateId:     "fabricS",
			ImportState:       true,
			ImportStateVerify: true,
		}}},
		{"set, change and unset route filter", []resource.TestStep{{
			Config: te.RenderConfig(`
				resource "proxmox_sdn_prefix_list" "first" {
				  id      = "test-rf-of-a"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/16", le = 32 }]
				}

				resource "proxmox_sdn_prefix_list" "second" {
				  id      = "test-rf-of-b"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/8", le = 32 }]
				}

				resource "proxmox_sdn_fabric_openfabric" "fabric" {
				  id        = "fabricSF"
				  ip_prefix = "10.0.0.0/16"
				  route_filter = proxmox_sdn_prefix_list.first.id
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_sdn_fabric_openfabric.fabric", map[string]string{"route_filter": "test-rf-of-a"}),
				checkFabricRouteFilter(te, fabrics.ProtocolOpenFabric, "fabricSF", "test-rf-of-a"),
			),
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_sdn_prefix_list" "first" {
				  id      = "test-rf-of-a"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/16", le = 32 }]
				}

				resource "proxmox_sdn_prefix_list" "second" {
				  id      = "test-rf-of-b"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/8", le = 32 }]
				}

				resource "proxmox_sdn_fabric_openfabric" "fabric" {
				  id        = "fabricSF"
				  ip_prefix = "10.0.0.0/16"
				  route_filter = proxmox_sdn_prefix_list.second.id
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_sdn_fabric_openfabric.fabric", map[string]string{"route_filter": "test-rf-of-b"}),
				checkFabricRouteFilter(te, fabrics.ProtocolOpenFabric, "fabricSF", "test-rf-of-b"),
			),
		}, {
			ResourceName:      "proxmox_sdn_fabric_openfabric.fabric",
			ImportStateId:     "fabricSF",
			ImportState:       true,
			ImportStateVerify: true,
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_sdn_prefix_list" "first" {
				  id      = "test-rf-of-a"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/16", le = 32 }]
				}

				resource "proxmox_sdn_prefix_list" "second" {
				  id      = "test-rf-of-b"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/8", le = 32 }]
				}

				resource "proxmox_sdn_fabric_openfabric" "fabric" {
				  id        = "fabricSF"
				  ip_prefix = "10.0.0.0/16"
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				NoResourceAttributesSet("proxmox_sdn_fabric_openfabric.fabric", []string{"route_filter"}),
				checkFabricRouteFilter(te, fabrics.ProtocolOpenFabric, "fabricSF", ""),
			),
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

func TestAccResourceSDNFabricOSPF(t *testing.T) {
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
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_virtual_environment_sdn_fabric_ospf.fabric_ospf", map[string]string{
					"id":   "fabricO",
					"area": "0",
				}),
			),
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_virtual_environment_sdn_fabric_ospf" "fabric_ospf" {
				  id        = "fabricO"
				  area      = "0"
				  ip_prefix = "10.0.1.0/16"
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_virtual_environment_sdn_fabric_ospf.fabric_ospf", map[string]string{
					"id":        "fabricO",
					"area":      "0",
					"ip_prefix": "10.0.1.0/16",
				}),
			),
			ResourceName:      "proxmox_virtual_environment_sdn_fabric_ospf.fabric_ospf",
			ImportStateId:     "fabricO",
			ImportState:       true,
			ImportStateVerify: true,
		}}},
		{"set, change and unset route filter", []resource.TestStep{{
			Config: te.RenderConfig(`
				resource "proxmox_sdn_prefix_list" "first" {
				  id      = "test-rf-ospf-a"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/16", le = 32 }]
				}

				resource "proxmox_sdn_prefix_list" "second" {
				  id      = "test-rf-ospf-b"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/8", le = 32 }]
				}

				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOF"
				  ip_prefix = "10.0.0.0/16"
				  area      = "0"
				  route_filter = proxmox_sdn_prefix_list.first.id
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_sdn_fabric_ospf.fabric", map[string]string{"route_filter": "test-rf-ospf-a"}),
				checkFabricRouteFilter(te, fabrics.ProtocolOSPF, "fabricOF", "test-rf-ospf-a"),
			),
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_sdn_prefix_list" "first" {
				  id      = "test-rf-ospf-a"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/16", le = 32 }]
				}

				resource "proxmox_sdn_prefix_list" "second" {
				  id      = "test-rf-ospf-b"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/8", le = 32 }]
				}

				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOF"
				  ip_prefix = "10.0.0.0/16"
				  area      = "0"
				  route_filter = proxmox_sdn_prefix_list.second.id
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				ResourceAttributes("proxmox_sdn_fabric_ospf.fabric", map[string]string{"route_filter": "test-rf-ospf-b"}),
				checkFabricRouteFilter(te, fabrics.ProtocolOSPF, "fabricOF", "test-rf-ospf-b"),
			),
		}, {
			ResourceName:      "proxmox_sdn_fabric_ospf.fabric",
			ImportStateId:     "fabricOF",
			ImportState:       true,
			ImportStateVerify: true,
		}, {
			Config: te.RenderConfig(`
				resource "proxmox_sdn_prefix_list" "first" {
				  id      = "test-rf-ospf-a"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/16", le = 32 }]
				}

				resource "proxmox_sdn_prefix_list" "second" {
				  id      = "test-rf-ospf-b"
				  entries = [{ seq = 5, action = "permit", prefix = "10.0.0.0/8", le = 32 }]
				}

				resource "proxmox_sdn_fabric_ospf" "fabric" {
				  id        = "fabricOF"
				  ip_prefix = "10.0.0.0/16"
				  area      = "0"
				}
			`),
			Check: resource.ComposeTestCheckFunc(
				NoResourceAttributesSet("proxmox_sdn_fabric_ospf.fabric", []string{"route_filter"}),
				checkFabricRouteFilter(te, fabrics.ProtocolOSPF, "fabricOF", ""),
			),
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
