//go:build acceptance || all

//testacc:tier=light
//testacc:resource=sdn

/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package prefix_list_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/bpg/terraform-provider-proxmox/fwprovider/test"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster"
)

// checkAPIEntries asserts the entries PVE holds for the prefix list, independent of Terraform state.
func checkAPIEntries(te *test.Environment, id string, want ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		client := &cluster.Client{Client: te.Client()}

		data, err := client.SDNPrefixLists().GetPrefixList(context.Background(), id)
		if err != nil {
			return fmt.Errorf("reading prefix list %q: %w", id, err)
		}

		if !slices.Equal(data.Entries, want) {
			return fmt.Errorf("prefix list %q entries: got %v, want %v", id, data.Entries, want)
		}

		return nil
	}
}

func TestAccResourceSDNPrefixList(t *testing.T) {
	te := test.InitEnvironment(t)

	tests := []struct {
		name  string
		steps []resource.TestStep
	}{
		{"create, replace entries and import", []resource.TestStep{{
			Config: te.RenderConfig(`
			resource "proxmox_sdn_prefix_list" "test" {
				id = "test-plist-a"
				entries = [
					{ seq = 5, action = "permit", prefix = "10.0.0.0/16", le = 32 },
					{ seq = 10, action = "permit", prefix = "10.254.254.0/24", le = 32 },
				]
			}`),
			Check: resource.ComposeTestCheckFunc(
				test.ResourceAttributes("proxmox_sdn_prefix_list.test", map[string]string{
					"id":               "test-plist-a",
					"entries.#":        "2",
					"entries.0.seq":    "5",
					"entries.0.action": "permit",
					"entries.0.prefix": "10.0.0.0/16",
					"entries.0.le":     "32",
					"entries.1.seq":    "10",
					"entries.1.prefix": "10.254.254.0/24",
				}),
				test.NoResourceAttributesSet("proxmox_sdn_prefix_list.test", []string{"entries.0.ge"}),
				checkAPIEntries(te, "test-plist-a",
					"action=permit,prefix=10.0.0.0/16,le=32,seq=5",
					"action=permit,prefix=10.254.254.0/24,le=32,seq=10",
				),
			),
		}, {
			// one entry changed, one removed, one added: PUT must replace the whole list
			Config: te.RenderConfig(`
			resource "proxmox_sdn_prefix_list" "test" {
				id = "test-plist-a"
				entries = [
					{ seq = 5, action = "deny", prefix = "10.0.0.0/16", ge = 24, le = 28 },
					{ seq = 20, action = "permit", prefix = "2001:db8::/32", le = 64 },
				]
			}`),
			Check: resource.ComposeTestCheckFunc(
				test.ResourceAttributes("proxmox_sdn_prefix_list.test", map[string]string{
					"entries.#":        "2",
					"entries.0.action": "deny",
					"entries.0.ge":     "24",
					"entries.0.le":     "28",
					"entries.1.seq":    "20",
					"entries.1.prefix": "2001:db8::/32",
					"entries.1.le":     "64",
				}),
				checkAPIEntries(te, "test-plist-a",
					"action=deny,prefix=10.0.0.0/16,ge=24,le=28,seq=5",
					"action=permit,prefix=2001:db8::/32,le=64,seq=20",
				),
			),
		}, {
			ResourceName:      "proxmox_sdn_prefix_list.test",
			ImportStateId:     "test-plist-a",
			ImportState:       true,
			ImportStateVerify: true,
		}}},
		{"exact-match entry without seq, ge or le", []resource.TestStep{{
			Config: te.RenderConfig(`
			resource "proxmox_sdn_prefix_list" "test" {
				id = "test-plist-b"
				entries = [
					{ action = "permit", prefix = "192.0.2.0/24" },
				]
			}`),
			Check: resource.ComposeTestCheckFunc(
				test.ResourceAttributes("proxmox_sdn_prefix_list.test", map[string]string{
					"entries.#":        "1",
					"entries.0.action": "permit",
					"entries.0.prefix": "192.0.2.0/24",
				}),
				test.NoResourceAttributesSet("proxmox_sdn_prefix_list.test", []string{"entries.0.ge", "entries.0.le"}),
			),
		}}},
		{"remove all entries", []resource.TestStep{{
			Config: te.RenderConfig(`
			resource "proxmox_sdn_prefix_list" "test" {
				id = "test-plist-c"
				entries = [
					{ seq = 5, action = "permit", prefix = "10.0.0.0/8", le = 32 },
				]
			}`),
			Check: test.ResourceAttributes("proxmox_sdn_prefix_list.test", map[string]string{"entries.#": "1"}),
		}, {
			Config: te.RenderConfig(`
			resource "proxmox_sdn_prefix_list" "test" {
				id = "test-plist-c"
			}`),
			Check: resource.ComposeTestCheckFunc(
				test.NoResourceAttributesSet("proxmox_sdn_prefix_list.test", []string{"entries.#"}),
				checkAPIEntries(te, "test-plist-c"),
			),
		}}},
		{"applied with the SDN applier", []resource.TestStep{{
			Config: te.RenderConfig(`
			resource "proxmox_sdn_applier" "finalizer" {}

			resource "proxmox_sdn_prefix_list" "test" {
				id = "test-plist-d"
				entries = [
					{ seq = 5, action = "permit", prefix = "10.0.0.0/16", le = 32 },
				]
				depends_on = [proxmox_sdn_applier.finalizer]
			}

			resource "proxmox_sdn_applier" "applier" {
				depends_on = [proxmox_sdn_prefix_list.test]
			}`),
			Check: checkAPIEntries(te, "test-plist-d", "action=permit,prefix=10.0.0.0/16,le=32,seq=5"),
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
