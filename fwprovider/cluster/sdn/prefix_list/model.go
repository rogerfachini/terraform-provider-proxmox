/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package prefix_list

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bpg/terraform-provider-proxmox/fwprovider/attribute"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster/sdn/prefix_lists"
)

type model struct {
	ID      types.String `tfsdk:"id"`
	Entries []entryModel `tfsdk:"entries"`
}

type entryModel struct {
	Seq    types.Int64  `tfsdk:"seq"`
	Action types.String `tfsdk:"action"`
	Prefix types.String `tfsdk:"prefix"`
	GE     types.Int64  `tfsdk:"ge"`
	LE     types.Int64  `tfsdk:"le"`
}

func (m *model) fromAPI(data *prefix_lists.PrefixListData, diags *diag.Diagnostics) {
	m.ID = types.StringValue(data.ID)
	m.Entries = nil

	for _, raw := range data.Entries {
		e, err := prefix_lists.ParseEntry(raw)
		if err != nil {
			diags.AddError("Unable to Parse SDN Prefix List Entry", err.Error())
			return
		}

		m.Entries = append(m.Entries, entryModel{
			Seq:    types.Int64PointerValue(e.Seq),
			Action: types.StringValue(e.Action),
			Prefix: types.StringValue(e.Prefix),
			GE:     types.Int64PointerValue(e.GE),
			LE:     types.Int64PointerValue(e.LE),
		})
	}
}

func (m *model) entriesToAPI() []string {
	entries := make([]string, 0, len(m.Entries))

	for _, e := range m.Entries {
		entries = append(entries, prefix_lists.Entry{
			Seq:    attribute.Int64PtrFromValue(e.Seq),
			Action: e.Action.ValueString(),
			Prefix: e.Prefix.ValueString(),
			GE:     attribute.Int64PtrFromValue(e.GE),
			LE:     attribute.Int64PtrFromValue(e.LE),
		}.String())
	}

	return entries
}
