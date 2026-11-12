/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package network

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bpg/terraform-provider-proxmox/fwprovider/config"
	"github.com/bpg/terraform-provider-proxmox/fwprovider/validators"
	"github.com/bpg/terraform-provider-proxmox/proxmox"
)

var (
	_ datasource.DataSource              = &interfacesDataSource{}
	_ datasource.DataSourceWithConfigure = &interfacesDataSource{}
)

type interfacesDataSource struct {
	client proxmox.Client
}

type interfacesDataSourceModel struct {
	NodeName   types.String            `tfsdk:"node_name"`
	Type       types.String            `tfsdk:"type"`
	Filters    *interfacesFiltersModel `tfsdk:"filters"`
	Interfaces []interfaceModel        `tfsdk:"interfaces"`
}

type interfacesFiltersModel struct {
	Name   types.String `tfsdk:"name"`
	Active types.Bool   `tfsdk:"active"`
}

type interfaceModel struct {
	Name            types.String   `tfsdk:"name"`
	Type            types.String   `tfsdk:"type"`
	Active          types.Bool     `tfsdk:"active"`
	Exists          types.Bool     `tfsdk:"exists"`
	Autostart       types.Bool     `tfsdk:"autostart"`
	Priority        types.Int64    `tfsdk:"priority"`
	Method          types.String   `tfsdk:"method"`
	Method6         types.String   `tfsdk:"method6"`
	Address         types.String   `tfsdk:"address"`
	Gateway         types.String   `tfsdk:"gateway"`
	Address6        types.String   `tfsdk:"address6"`
	Gateway6        types.String   `tfsdk:"gateway6"`
	MTU             types.Int64    `tfsdk:"mtu"`
	Comment         types.String   `tfsdk:"comment"`
	Altnames        []types.String `tfsdk:"altnames"`
	BridgePorts     []types.String `tfsdk:"bridge_ports"`
	BridgeVLANAware types.Bool     `tfsdk:"bridge_vlan_aware"`
	BridgeVIDs      types.String   `tfsdk:"bridge_vids"`
	BondSlaves      []types.String `tfsdk:"bond_slaves"`
	BondMode        types.String   `tfsdk:"bond_mode"`
	VLANID          types.Int64    `tfsdk:"vlan_id"`
	VLANRawDevice   types.String   `tfsdk:"vlan_raw_device"`
	Options         []types.String `tfsdk:"options"`
	Options6        []types.String `tfsdk:"options6"`
}

// NewInterfacesDataSource creates the proxmox_network_interfaces data source.
func NewInterfacesDataSource() datasource.DataSource {
	return &interfacesDataSource{}
}

func (d *interfacesDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "proxmox_network_interfaces"
}

func (d *interfacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computedString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Description: desc, Computed: true}
	}
	computedBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Description: desc, Computed: true}
	}
	computedInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Description: desc, Computed: true}
	}
	computedStrings := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{Description: desc, Computed: true, ElementType: types.StringType}
	}

	resp.Schema = schema.Schema{
		Description: "Retrieves the network interfaces of a Proxmox VE node.",
		MarkdownDescription: "Retrieves the network interfaces of a Proxmox VE node, as found in its network configuration. " +
			"This is useful for discovering physical interfaces, or for checking that an interface exists " +
			"before referencing it from a bridge, bond, VLAN or SDN fabric.",
		Attributes: map[string]schema.Attribute{
			"node_name": schema.StringAttribute{
				Description: "The name of the node.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description: "Only list interfaces of this type. The filter is applied by Proxmox VE.",
				MarkdownDescription: "Only list interfaces of this type. The filter is applied by Proxmox VE. Besides the " +
					"interface types (`eth`, `bridge`, `bond`, `vlan`, `alias`, `fabric`, `vnet`, `OVSBridge`, `OVSBond`, " +
					"`OVSPort`, `OVSIntPort`), `any_bridge` lists Linux, OVS and SDN bridges, `any_local_bridge` lists Linux " +
					"and OVS bridges, and `include_sdn` lists all interfaces together with SDN VNets.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf(
						"bridge", "bond", "eth", "alias", "vlan", "fabric", "OVSBridge", "OVSBond", "OVSPort", "OVSIntPort",
						"vnet", "any_bridge", "any_local_bridge", "include_sdn",
					),
				},
			},
			"filters": schema.SingleNestedAttribute{
				Description: "Client-side filters for narrowing down results. An interface must match all filters that are set.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Description: "A regular expression the interface name must match.",
						MarkdownDescription: "A regular expression (RE2 syntax) the interface name must match, " +
							"e.g. `^nic[0-9]+$`.",
						Optional: true,
						Validators: []validator.String{
							validators.NewParseValidator(regexp.Compile, "must be a valid regular expression"),
						},
					},
					"active": schema.BoolAttribute{
						Description: "Whether the interface must be active (`true`) or inactive (`false`).",
						Optional:    true,
					},
				},
			},
			"interfaces": schema.ListNestedAttribute{
				Description: "The network interfaces, ordered by their position in the node's network configuration.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":      computedString("The interface name."),
						"type":      computedString("The interface type, e.g. `eth`, `bridge`, `bond` or `vlan`."),
						"active":    computedBool("Whether the interface is active."),
						"exists":    computedBool("Whether the interface physically exists."),
						"autostart": computedBool("Whether the interface is started automatically on boot."),
						"priority":  computedInt("The position of the interface in the node's network configuration."),
						"method":    computedString("The IPv4 configuration method, e.g. `manual` or `static`."),
						"method6":   computedString("The IPv6 configuration method, e.g. `manual` or `static`."),
						"address":   computedString("The interface IPv4/CIDR address."),
						"gateway":   computedString("The default IPv4 gateway address."),
						"address6":  computedString("The interface IPv6/CIDR address."),
						"gateway6":  computedString("The default IPv6 gateway address."),
						"mtu":       computedInt("The interface MTU, when one is configured."),
						"comment":   computedString("The comment of the interface."),
						"altnames":  computedStrings("The alternative names of the interface."),
						"bridge_ports": computedStrings(
							"The ports of the bridge."),
						"bridge_vlan_aware": computedBool("Whether the bridge is VLAN aware."),
						"bridge_vids":       computedString("The VLAN IDs allowed on the bridge."),
						"bond_slaves":       computedStrings("The member interfaces of the bond."),
						"bond_mode":         computedString("The bonding mode."),
						"vlan_id":           computedInt("The VLAN tag, when the interface is a VLAN."),
						"vlan_raw_device":   computedString("The raw device of the VLAN."),
						"options": computedStrings(
							"Additional IPv4 options of the interface that Proxmox VE does not manage, " +
								"e.g. `bridge-stp on` or `post-up` commands."),
						"options6": computedStrings(
							"Additional IPv6 options of the interface that Proxmox VE does not manage."),
					},
				},
			},
		},
	}
}

func (d *interfacesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(config.DataSource)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected config.DataSource, got: %T", req.ProviderData),
		)

		return
	}

	d.client = cfg.Client
}

func (d *interfacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model interfacesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)

	if resp.Diagnostics.HasError() {
		return
	}

	nodeName := model.NodeName.ValueString()

	ifaces, err := d.client.Node(nodeName).ListNetworkInterfacesOfType(ctx, model.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to List Network Interfaces of Node %q", nodeName), err.Error())
		return
	}

	var nameFilter *regexp.Regexp

	if model.Filters != nil && !model.Filters.Name.IsNull() {
		nameFilter, err = regexp.Compile(model.Filters.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid Interface Name Filter", err.Error())
			return
		}
	}

	model.Interfaces = make([]interfaceModel, 0, len(ifaces))

	for _, iface := range ifaces {
		active := iface.Active != nil && bool(*iface.Active)

		if nameFilter != nil && !nameFilter.MatchString(iface.Iface) {
			continue
		}

		if model.Filters != nil && !model.Filters.Active.IsNull() && model.Filters.Active.ValueBool() != active {
			continue
		}

		model.Interfaces = append(model.Interfaces, interfaceModel{
			Name:            types.StringValue(iface.Iface),
			Type:            types.StringValue(iface.Type),
			Active:          types.BoolValue(active),
			Exists:          types.BoolValue(iface.Exists != nil && bool(*iface.Exists)),
			Autostart:       types.BoolValue(iface.Autostart != nil && bool(*iface.Autostart)),
			Priority:        types.Int64Value(int64(iface.Priority)),
			Method:          stringOrEmpty(iface.MethodIPv4),
			Method6:         stringOrEmpty(iface.MethodIPv6),
			Address:         stringOrEmpty(iface.CIDR),
			Gateway:         stringOrEmpty(iface.Gateway),
			Address6:        stringOrEmpty(iface.CIDR6),
			Gateway6:        stringOrEmpty(iface.Gateway6),
			MTU:             int64OrNull(iface.MTU),
			Comment:         stringOrEmpty(iface.Comments),
			Altnames:        stringList(iface.Altnames),
			BridgePorts:     fieldList(iface.BridgePorts),
			BridgeVLANAware: types.BoolValue(iface.BridgeVLANAware != nil && bool(*iface.BridgeVLANAware)),
			BridgeVIDs:      stringOrEmpty(iface.BridgeVIDs),
			BondSlaves:      fieldList(iface.Slaves),
			BondMode:        stringOrEmpty(iface.BondMode),
			VLANID:          int64OrNull(iface.VLANID),
			VLANRawDevice:   stringOrEmpty(iface.VLANRawDevice),
			Options:         stringList(iface.Options),
			Options6:        stringList(iface.Options6),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func stringOrEmpty(p *string) types.String {
	if p == nil {
		return types.StringValue("")
	}

	return types.StringValue(strings.TrimSpace(*p))
}

// int64OrNull converts the numeric strings PVE returns for mtu and vlan-id.
func int64OrNull(p *string) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}

	v, err := strconv.ParseInt(strings.TrimSpace(*p), 10, 64)
	if err != nil {
		return types.Int64Null()
	}

	return types.Int64Value(v)
}

func stringList(values []string) []types.String {
	out := make([]types.String, 0, len(values))
	for _, v := range values {
		out = append(out, types.StringValue(v))
	}

	return out
}

// fieldList splits the whitespace-separated interface lists PVE returns for bridge ports and bond slaves.
func fieldList(p *string) []types.String {
	if p == nil {
		return []types.String{}
	}

	return stringList(strings.Fields(*p))
}
