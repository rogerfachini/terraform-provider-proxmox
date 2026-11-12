/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package fabric_node

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bpg/terraform-provider-proxmox/fwprovider/attribute"
	"github.com/bpg/terraform-provider-proxmox/fwprovider/migration"
	customtypes "github.com/bpg/terraform-provider-proxmox/fwprovider/types"
	"github.com/bpg/terraform-provider-proxmox/fwprovider/validators"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster/sdn/fabric_nodes"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster/sdn/fabrics"
)

var (
	_ resource.ResourceWithConfigure   = &OSPFResource{}
	_ resource.ResourceWithImportState = &OSPFResource{}
)

type ospfModel struct {
	genericModel

	IPv4Address customtypes.IPAddrValue `tfsdk:"ip"`
	Interfaces  types.Set               `tfsdk:"interfaces"`
}

func (m *ospfModel) fromAPI(name string, data *fabric_nodes.FabricNodeData, diags *diag.Diagnostics) {
	m.genericModel.fromAPI(name, data, diags)

	m.IPv4Address = m.handleDeletedIPAddrValue(data.IPv4Address)

	m.Interfaces = ospfInterfacesFromAPI(data.Interfaces, diags)
}

func (m *ospfModel) toAPI(ctx context.Context, diags *diag.Diagnostics) *fabric_nodes.FabricNode {
	data := m.genericModel.toAPI(ctx, diags)

	data.IPv4Address = m.IPv4Address.ValueStringPointer()

	if attribute.IsDefined(m.Interfaces) {
		data.Interfaces = ospfInterfacesToAPI(ctx, m.Interfaces, diags)
	}

	return data
}

// shapeLike keeps whichever of interface_names and interfaces the prior plan or state used. On import the simpler
// interface_names is used unless an interface carries options only interfaces can express.
func (m *ospfModel) shapeLike(prior fabricNodeModel) {
	useInterfaces := ospfInterfacesHaveOptions(m.Interfaces)

	if p, ok := prior.(*ospfModel); ok && p != nil {
		useInterfaces = !p.Interfaces.IsNull()
	}

	if useInterfaces {
		m.InterfaceNames = types.SetNull(types.StringType)
	} else {
		m.Interfaces = types.SetNull(types.ObjectType{AttrTypes: ospfInterfaceAttrTypes()})
	}
}

func checkDeletedOspfFields(state, plan *ospfModel) []string {
	var toDelete []string

	if plan.IPv4Address.IsNull() && !state.IPv4Address.IsNull() {
		toDelete = append(toDelete, "ip")
	}

	if plan.Interfaces.IsNull() && !state.Interfaces.IsNull() {
		toDelete = append(toDelete, "interfaces")
	}

	return toDelete
}

type OSPFResource struct {
	*genericFabricNodeResource
}

func NewOSPFResource() resource.Resource {
	return &OSPFResource{
		genericFabricNodeResource: newGenericFabricNodeResource(fabricNodeResourceConfig{
			typeNameSuffix: "_sdn_fabric_node_ospf",
			fabricProtocol: fabrics.ProtocolOSPF,
			modelFunc:      func() fabricNodeModel { return &ospfModel{} },
		}).(*genericFabricNodeResource),
	}
}

func (r *OSPFResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ospfModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state ospfModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	toDelete := checkDeletedOspfFields(&state, &plan)
	r.update(ctx, req, resp, toDelete)
}

func (r *OSPFResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		DeprecationMessage:  migration.DeprecationMessage("proxmox_sdn_fabric_node_ospf"),
		Description:         "OSPF Fabric Node in Proxmox SDN. Fabrics in Proxmox VE SDN provide automated routing between nodes in a cluster.",
		MarkdownDescription: "OSPF Fabric Node in Proxmox SDN. Fabrics in Proxmox VE SDN provide automated routing between nodes in a cluster.",
		Attributes: genericAttributesWith(map[string]schema.Attribute{
			"ip": schema.StringAttribute{
				Description: "IPv4 address for the fabric node.",
				Required:    true,
				CustomType:  customtypes.IPAddrType{},
			},
			"interface_names": schema.SetAttribute{
				Description: "Set of interfaces associated with the fabric node. " +
					"Exactly one of `interface_names` and `interfaces` must be set.",
				ElementType: types.StringType,
				Optional:    true,
				Validators: []validator.Set{
					setvalidator.ExactlyOneOf(path.MatchRoot("interfaces")),
				},
			},
			"interfaces": schema.SetNestedAttribute{
				Description: "Set of interfaces associated with the fabric node, with per-interface OSPF options.",
				MarkdownDescription: "Set of interfaces associated with the fabric node, with per-interface OSPF options. " +
					"Use this instead of `interface_names` when an interface needs its own address or network type. " +
					"Exactly one of `interface_names` and `interfaces` must be set.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the network interface.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.RegexMatches(
									regexp.MustCompile(`^[^,=\s]+$`),
									"must be an interface name without commas, equal signs or whitespace",
								),
							},
						},
						"ip": schema.StringAttribute{
							Description:         "The IPv4 address of the interface in CIDR notation.",
							MarkdownDescription: "The IPv4 address of the interface in CIDR notation, e.g. `10.0.100.1/31`.",
							Optional:            true,
							Validators: []validator.String{
								validators.NewParseValidator(parseIPv4Prefix, "must be a valid IPv4 address in CIDR notation"),
							},
						},
						"network_type": schema.StringAttribute{
							Description: "The OSPF network type of the interface.",
							MarkdownDescription: "The OSPF network type of the interface. Must be one of `broadcast`, " +
								"`non-broadcast`, `point-to-multipoint` or `point-to-point`.",
							Optional: true,
							Validators: []validator.String{
								stringvalidator.OneOf("broadcast", "non-broadcast", "point-to-multipoint", "point-to-point"),
							},
						},
					},
				},
			},
		}),
	}
}

func parseIPv4Prefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return p, fmt.Errorf("%q is not a valid IPv4 prefix: %w", s, err)
	}

	if !p.Addr().Is4() {
		return p, fmt.Errorf("%q is not an IPv4 prefix", s)
	}

	return p, nil
}

func (m *ospfModel) getGenericModel() *genericModel {
	return &m.genericModel
}
