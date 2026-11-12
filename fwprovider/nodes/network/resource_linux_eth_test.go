//go:build acceptance || all

//testacc:tier=heavy
//testacc:resource=network

/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package network_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/bpg/terraform-provider-proxmox/fwprovider/test"
	"github.com/bpg/terraform-provider-proxmox/proxmox/nodes"
)

const accTestLinuxEthName = "proxmox_network_linux_eth.test"

// checkEthInterface runs a check against what PVE reports for the interface, independent of Terraform state.
func checkEthInterface(te *test.Environment, iface string, check func(*nodes.NetworkInterfaceListResponseData) error) func() error {
	return func() error {
		ifaces, err := te.NodeClient().ListNetworkInterfaces(context.Background())
		if err != nil {
			return fmt.Errorf("listing network interfaces: %w", err)
		}

		for _, i := range ifaces {
			if i.Iface == iface {
				return check(i)
			}
		}

		return fmt.Errorf("interface %q not found on node", iface)
	}
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}

	return *p
}

func wantMTU(want string) func(*nodes.NetworkInterfaceListResponseData) error {
	return func(i *nodes.NetworkInterfaceListResponseData) error {
		if str(i.MTU) != want {
			return fmt.Errorf("interface %q: mtu = %s, want %s", i.Iface, str(i.MTU), want)
		}

		return nil
	}
}

// TestAccResourceLinuxEth requires a free eth-type interface that carries no address.
//
// Set PROXMOX_VE_ACC_BOND_SLAVE1 in testacc.env; see TestAccResourceLinuxBond for how to provide one on a
// single-NIC test host.
func TestAccResourceLinuxEth(t *testing.T) {
	te := test.InitEnvironment(t)

	iface := os.Getenv("PROXMOX_VE_ACC_BOND_SLAVE1")
	if iface == "" {
		t.Skip("skipping: PROXMOX_VE_ACC_BOND_SLAVE1 must be set to a free eth-type interface")
	}

	ipV4cidr := fmt.Sprintf("%s/24", gofakeit.IPv4Address())

	apiCheck := func(check func(*nodes.NetworkInterfaceListResponseData) error) resource.TestCheckFunc {
		return func(_ *terraform.State) error { return checkEthInterface(te, iface, check)() }
	}

	// Sequential: a network reload applies all pending changes on the node.
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.AccProviders,
		// destroy must revert the managed settings and leave the interface itself in place
		CheckDestroy: func(_ *terraform.State) error {
			return checkEthInterface(te, iface, func(i *nodes.NetworkInterfaceListResponseData) error {
				if i.MTU != nil || i.Comments != nil || i.CIDR != nil {
					return fmt.Errorf("interface %q still configured after destroy: mtu=%s comments=%s cidr=%s",
						i.Iface, str(i.MTU), str(i.Comments), str(i.CIDR))
				}

				return nil
			})()
		},
		Steps: []resource.TestStep{
			{
				// adopt with MTU only
				Config: te.RenderConfig(fmt.Sprintf(`
				resource "proxmox_network_linux_eth" "test" {
					node_name = "{{.NodeName}}"
					name      = %q
					mtu       = 9000
				}`, iface)),
				Check: resource.ComposeTestCheckFunc(
					test.ResourceAttributes(accTestLinuxEthName, map[string]string{
						"id":     fmt.Sprintf("%s:%s", te.NodeName, iface),
						"name":   iface,
						"mtu":    "9000",
						"reload": "true",
					}),
					test.NoResourceAttributesSet(accTestLinuxEthName, []string{"address", "comment", "gateway"}),
					test.ResourceAttributesSet(accTestLinuxEthName, []string{"autostart"}),
					apiCheck(wantMTU("9000")),
				),
			},
			{
				// change MTU, add comment, autostart and an address
				Config: te.RenderConfig(fmt.Sprintf(`
				resource "proxmox_network_linux_eth" "test" {
					node_name = "{{.NodeName}}"
					name      = %q
					mtu       = 1600
					comment   = "managed by terraform"
					autostart = true
					address   = %q
				}`, iface, ipV4cidr)),
				Check: resource.ComposeTestCheckFunc(
					test.ResourceAttributes(accTestLinuxEthName, map[string]string{
						"mtu":       "1600",
						"comment":   "managed by terraform",
						"autostart": "true",
						"address":   ipV4cidr,
					}),
					apiCheck(wantMTU("1600")),
					apiCheck(func(i *nodes.NetworkInterfaceListResponseData) error {
						if str(i.CIDR) != ipV4cidr {
							return fmt.Errorf("cidr = %s, want %s", str(i.CIDR), ipV4cidr)
						}

						return nil
					}),
				),
			},
			{
				ResourceName:            accTestLinuxEthName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeout_reload"},
			},
			{
				// drop MTU, comment and address: each must be deleted on the node
				Config: te.RenderConfig(fmt.Sprintf(`
				resource "proxmox_network_linux_eth" "test" {
					node_name = "{{.NodeName}}"
					name      = %q
					autostart = false
				}`, iface)),
				Check: resource.ComposeTestCheckFunc(
					test.ResourceAttributes(accTestLinuxEthName, map[string]string{"autostart": "false"}),
					test.NoResourceAttributesSet(accTestLinuxEthName, []string{"mtu", "comment", "address"}),
					apiCheck(func(i *nodes.NetworkInterfaceListResponseData) error {
						if i.MTU != nil || i.Comments != nil || i.CIDR != nil {
							return fmt.Errorf("mtu=%s comments=%s cidr=%s, want all unset", str(i.MTU), str(i.Comments), str(i.CIDR))
						}

						return nil
					}),
				),
			},
			{
				// staged only: the MTU is written to the pending configuration but not applied
				Config: te.RenderConfig(fmt.Sprintf(`
				resource "proxmox_network_linux_eth" "test" {
					node_name = "{{.NodeName}}"
					name      = %q
					mtu       = 9000
					reload    = false
				}`, iface)),
				Check: resource.ComposeTestCheckFunc(
					test.ResourceAttributes(accTestLinuxEthName, map[string]string{"mtu": "9000", "reload": "false"}),
					checkNodePendingChanges(te, true),
					reloadNode(te),
				),
			},
		},
	})
}

func TestAccResourceLinuxEthRejectsNonEthInterfaces(t *testing.T) {
	te := test.InitEnvironment(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.AccProviders,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
				resource "proxmox_network_linux_eth" "test" {
					node_name = "{{.NodeName}}"
					name      = "nonexistent0"
					mtu       = 9000
				}`),
				ExpectError: regexp.MustCompile(`has no physical \(eth type\) network interface named "nonexistent0"`),
			},
			{
				// a bridge is not an eth interface
				Config: te.RenderConfig(`
				resource "proxmox_network_linux_eth" "test" {
					node_name = "{{.NodeName}}"
					name      = "vmbr0"
					mtu       = 9000
				}`),
				ExpectError: regexp.MustCompile(`has no physical \(eth type\) network interface named "vmbr0"`),
			},
		},
	})
}

// An update without an address resets the interface to manual addressing, so adopting an interface whose
// address is not in the configuration must fail before anything is sent to the node.
func TestAccResourceLinuxEthRejectsUnmanagedAddress(t *testing.T) {
	te := test.InitEnvironment(t)

	iface := os.Getenv("PROXMOX_VE_ACC_BOND_SLAVE1")
	if iface == "" {
		t.Skip("skipping: PROXMOX_VE_ACC_BOND_SLAVE1 must be set to a free eth-type interface")
	}

	cidr := fmt.Sprintf("%s/24", gofakeit.IPv4Address())
	ctx := context.Background()

	// staged only, never reloaded: the address does not reach the kernel
	err := te.NodeClient().UpdateNetworkInterface(ctx, iface, &nodes.NetworkInterfaceCreateUpdateRequestBody{
		Iface: iface, Type: "eth", CIDR: &cidr,
	})
	if err != nil {
		t.Fatalf("staging address on %s: %v", iface, err)
	}

	t.Cleanup(func() {
		err := te.NodeClient().UpdateNetworkInterface(ctx, iface, &nodes.NetworkInterfaceCreateUpdateRequestBody{
			Iface: iface, Type: "eth", Delete: []string{"cidr"},
		})
		if err != nil {
			t.Logf("cleanup: removing staged address from %s: %v", iface, err)
		}
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.AccProviders,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(fmt.Sprintf(`
				resource "proxmox_network_linux_eth" "test" {
					node_name = "{{.NodeName}}"
					name      = %q
					mtu       = 9000
				}`, iface)),
				ExpectError: regexp.MustCompile(`already has address`),
			},
			{
				// the address survived the rejected create
				Config: te.RenderConfig(fmt.Sprintf(`
				resource "proxmox_network_linux_eth" "test" {
					node_name = "{{.NodeName}}"
					name      = %q
					address   = %q
					reload    = false
				}`, iface, cidr)),
				Check: test.ResourceAttributes(accTestLinuxEthName, map[string]string{"address": cidr}),
			},
		},
	})
}
