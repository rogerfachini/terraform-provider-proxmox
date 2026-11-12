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
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/bpg/terraform-provider-proxmox/fwprovider/test"
)

const accTestInterfacesName = "data.proxmox_network_interfaces.test"

// checkEveryInterface asserts that the data source returned at least one interface and that each one passes.
func checkEveryInterface(attr string, ok func(string) bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, found := s.RootModule().Resources[accTestInterfacesName]
		if !found {
			return fmt.Errorf("%s not found in state", accTestInterfacesName)
		}

		count, err := strconv.Atoi(rs.Primary.Attributes["interfaces.#"])
		if err != nil || count == 0 {
			return fmt.Errorf("interfaces.# = %q, want at least one", rs.Primary.Attributes["interfaces.#"])
		}

		for i := range count {
			key := fmt.Sprintf("interfaces.%d.%s", i, attr)
			if v := rs.Primary.Attributes[key]; !ok(v) {
				return fmt.Errorf("%s = %q fails the check", key, v)
			}
		}

		return nil
	}
}

// checkInterfaceListed asserts that an interface with the given attributes is among the results.
func checkInterfaceListed(name string, attrs map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, found := s.RootModule().Resources[accTestInterfacesName]
		if !found {
			return fmt.Errorf("%s not found in state", accTestInterfacesName)
		}

		count, _ := strconv.Atoi(rs.Primary.Attributes["interfaces.#"])

		for i := range count {
			prefix := fmt.Sprintf("interfaces.%d.", i)
			if rs.Primary.Attributes[prefix+"name"] != name {
				continue
			}

			for k, want := range attrs {
				if got := rs.Primary.Attributes[prefix+k]; got != want {
					return fmt.Errorf("interface %q: %s = %q, want %q", name, k, got, want)
				}
			}

			return nil
		}

		return fmt.Errorf("interface %q not listed (%d interfaces returned)", name, count)
	}
}

func TestAccDataSourceNetworkInterfaces(t *testing.T) {
	te := test.InitEnvironment(t)

	bridge := fmt.Sprintf("vmbr%d", gofakeit.Number(10, 9999))
	cidr := fmt.Sprintf("%s/24", gofakeit.IPv4Address())

	bridgeConfig := fmt.Sprintf(`
	resource "proxmox_network_linux_bridge" "test" {
		node_name  = "{{.NodeName}}"
		name       = %q
		address    = %q
		comment    = "interfaces data source test"
		mtu        = 1400
		vlan_aware = true
	}
	`, bridge, cidr)

	tests := []struct {
		name  string
		steps []resource.TestStep
	}{
		{"list all interfaces", []resource.TestStep{{
			Config: te.RenderConfig(`data "proxmox_network_interfaces" "test" {
				node_name = "{{.NodeName}}"
			}`),
			Check: resource.ComposeTestCheckFunc(
				checkEveryInterface("name", func(v string) bool { return v != "" }),
				checkEveryInterface("type", func(v string) bool { return v != "" }),
				// regression guard: the unfiltered list must include bridges
				checkInterfaceListed("vmbr0", map[string]string{"type": "bridge"}),
			),
		}}},
		{"server-side type filter", []resource.TestStep{{
			Config: te.RenderConfig(`data "proxmox_network_interfaces" "test" {
				node_name = "{{.NodeName}}"
				type      = "eth"
			}`),
			Check: checkEveryInterface("type", func(v string) bool { return v == "eth" }),
		}, {
			Config: te.RenderConfig(`data "proxmox_network_interfaces" "test" {
				node_name = "{{.NodeName}}"
				type      = "any_bridge"
			}`),
			Check: checkInterfaceListed("vmbr0", map[string]string{"type": "bridge"}),
		}}},
		{"attributes of a created bridge", []resource.TestStep{{
			Config: te.RenderConfig(bridgeConfig + `
			data "proxmox_network_interfaces" "test" {
				node_name = "{{.NodeName}}"
				type      = "bridge"

				depends_on = [proxmox_network_linux_bridge.test]
			}`),
			Check: checkInterfaceListed(bridge, map[string]string{
				"type":              "bridge",
				"address":           cidr,
				"method":            "static",
				"comment":           "interfaces data source test",
				"mtu":               "1400",
				"autostart":         "true",
				"bridge_vlan_aware": "true",
				"bridge_ports.#":    "0",
			}),
		}}},
		{"client-side filters", []resource.TestStep{{
			Config: te.RenderConfig(bridgeConfig + fmt.Sprintf(`
			data "proxmox_network_interfaces" "test" {
				node_name = "{{.NodeName}}"

				filters = {
					name   = "^%s$"
					active = true
				}

				depends_on = [proxmox_network_linux_bridge.test]
			}`, bridge)),
			Check: resource.ComposeTestCheckFunc(
				test.ResourceAttributes(accTestInterfacesName, map[string]string{
					"interfaces.#":      "1",
					"interfaces.0.name": bridge,
				}),
			),
		}, {
			Config: te.RenderConfig(`data "proxmox_network_interfaces" "test" {
				node_name = "{{.NodeName}}"

				filters = {
					name = "^no-such-interface$"
				}
			}`),
			Check: test.ResourceAttributes(accTestInterfacesName, map[string]string{"interfaces.#": "0"}),
		}, {
			Config: te.RenderConfig(`data "proxmox_network_interfaces" "test" {
				node_name = "{{.NodeName}}"

				filters = {
					name = "("
				}
			}`),
			ExpectError: regexp.MustCompile(`must be a valid regular expression`),
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sequential: the bridge cases reload the node network configuration.
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: te.AccProviders,
				Steps:                    tt.steps,
			})
		})
	}
}
