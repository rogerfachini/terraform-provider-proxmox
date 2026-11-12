/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package fabric_node

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ospfInterfaceModel is one entry of the PVE `interfaces` array of an OSPF fabric node, which the API carries as a
// property string, e.g. "name=nic0,ip=10.0.100.1/31,network_type=point-to-point".
type ospfInterfaceModel struct {
	Name        types.String `tfsdk:"name"`
	IP          types.String `tfsdk:"ip"`
	NetworkType types.String `tfsdk:"network_type"`
}

func ospfInterfaceAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":         types.StringType,
		"ip":           types.StringType,
		"network_type": types.StringType,
	}
}

func (m ospfInterfaceModel) hasOptions() bool {
	return !m.IP.IsNull() || !m.NetworkType.IsNull()
}

func (m ospfInterfaceModel) toAPI() string {
	parts := []string{"name=" + m.Name.ValueString()}

	if v := m.IP.ValueString(); v != "" {
		parts = append(parts, "ip="+v)
	}

	if v := m.NetworkType.ValueString(); v != "" {
		parts = append(parts, "network_type="+v)
	}

	return strings.Join(parts, ",")
}

func parseOSPFInterface(raw string) (ospfInterfaceModel, error) {
	m := ospfInterfaceModel{
		Name:        types.StringNull(),
		IP:          types.StringNull(),
		NetworkType: types.StringNull(),
	}

	for kv := range strings.SplitSeq(raw, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return m, fmt.Errorf("invalid fabric node interface %q: %q is not a key=value pair", raw, kv)
		}

		switch k {
		case "name":
			m.Name = types.StringValue(v)
		case "ip":
			m.IP = types.StringValue(v)
		case "network_type":
			m.NetworkType = types.StringValue(v)
		}
	}

	if m.Name.IsNull() {
		return m, fmt.Errorf("invalid fabric node interface %q: no name", raw)
	}

	return m, nil
}

func ospfInterfacesFromAPI(raw []string, diags *diag.Diagnostics) types.Set {
	objType := types.ObjectType{AttrTypes: ospfInterfaceAttrTypes()}
	values := make([]attr.Value, 0, len(raw))

	for _, r := range raw {
		m, err := parseOSPFInterface(r)
		if err != nil {
			diags.AddError("Unable to Parse SDN Fabric Node Interface", err.Error())
			return types.SetNull(objType)
		}

		obj, d := types.ObjectValueFrom(context.Background(), ospfInterfaceAttrTypes(), m)
		diags.Append(d...)

		values = append(values, obj)
	}

	set, d := types.SetValue(objType, values)
	diags.Append(d...)

	return set
}

// ospfInterfacesHaveOptions reports whether any interface carries options that interface_names cannot express.
func ospfInterfacesHaveOptions(set types.Set) bool {
	var models []ospfInterfaceModel

	if set.ElementsAs(context.Background(), &models, false).HasError() {
		return false
	}

	return slices.ContainsFunc(models, ospfInterfaceModel.hasOptions)
}

func ospfInterfacesToAPI(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	var models []ospfInterfaceModel

	diags.Append(set.ElementsAs(ctx, &models, false)...)

	if diags.HasError() {
		return nil
	}

	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, m.toAPI())
	}

	return out
}
