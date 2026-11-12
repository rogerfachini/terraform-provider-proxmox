/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bpg/terraform-provider-proxmox/fwprovider/attribute"
	"github.com/bpg/terraform-provider-proxmox/fwprovider/config"
	customtypes "github.com/bpg/terraform-provider-proxmox/fwprovider/types"
	"github.com/bpg/terraform-provider-proxmox/proxmox"
	"github.com/bpg/terraform-provider-proxmox/proxmox/nodes"
)

const linuxEthType = "eth"

var (
	_ resource.Resource                = &linuxEthResource{}
	_ resource.ResourceWithConfigure   = &linuxEthResource{}
	_ resource.ResourceWithImportState = &linuxEthResource{}
)

type linuxEthResourceModel struct {
	ID        types.String            `tfsdk:"id"`
	NodeName  types.String            `tfsdk:"node_name"`
	Name      types.String            `tfsdk:"name"`
	Address   customtypes.IPCIDRValue `tfsdk:"address"`
	Gateway   customtypes.IPAddrValue `tfsdk:"gateway"`
	Address6  customtypes.IPCIDRValue `tfsdk:"address6"`
	Gateway6  customtypes.IPAddrValue `tfsdk:"gateway6"`
	Autostart types.Bool              `tfsdk:"autostart"`
	MTU       types.Int64             `tfsdk:"mtu"`
	Comment   types.String            `tfsdk:"comment"`
	Timeout   types.Int64             `tfsdk:"timeout_reload"`
	Reload    types.Bool              `tfsdk:"reload"`
}

func (m *linuxEthResourceModel) exportToNetworkInterfaceCreateUpdateBody() *nodes.NetworkInterfaceCreateUpdateRequestBody {
	body := &nodes.NetworkInterfaceCreateUpdateRequestBody{
		Iface: m.Name.ValueString(),
		Type:  linuxEthType,
	}

	body.Autostart = attribute.CustomBoolPtrFromValue(m.Autostart)
	body.CIDR = m.Address.ValueStringPointer()
	body.Gateway = m.Gateway.ValueStringPointer()
	body.CIDR6 = m.Address6.ValueStringPointer()
	body.Gateway6 = m.Gateway6.ValueStringPointer()
	body.Comments = attribute.StringPtrFromValue(m.Comment)
	body.MTU = attribute.Int64PtrFromValue(m.MTU)

	return body
}

func (m *linuxEthResourceModel) importFromNetworkInterfaceList(iface *nodes.NetworkInterfaceListResponseData) {
	m.Address = customtypes.NewIPCIDRPointerValue(iface.CIDR)
	m.Gateway = customtypes.NewIPAddrPointerValue(iface.Gateway)
	m.Address6 = customtypes.NewIPCIDRPointerValue(iface.CIDR6)
	m.Gateway6 = customtypes.NewIPAddrPointerValue(iface.Gateway6)
	// PVE omits autostart for interfaces without an `auto` line
	m.Autostart = types.BoolValue(iface.Autostart != nil && bool(*iface.Autostart))

	m.MTU = types.Int64Null()

	if iface.MTU != nil {
		if v, err := strconv.Atoi(*iface.MTU); err == nil {
			m.MTU = types.Int64Value(int64(v))
		}
	}

	if iface.Comments != nil {
		m.Comment = types.StringValue(strings.TrimSpace(*iface.Comments))
	} else {
		m.Comment = types.StringNull()
	}
}

// NewLinuxEthResource creates a new resource for managing the configuration of physical network interfaces.
func NewLinuxEthResource() resource.Resource {
	return &linuxEthResource{}
}

type linuxEthResource struct {
	client proxmox.Client
}

func (r *linuxEthResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "proxmox_network_linux_eth"
}

func (r *linuxEthResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the configuration of a physical network interface in a Proxmox VE node.",
		MarkdownDescription: "Manages the configuration of a physical (`eth` type) network interface in a Proxmox VE node.\n\n" +
			"The interface itself is hardware and is neither created nor removed by this resource. Creating the resource " +
			"adopts the existing interface and applies the configured settings; destroying it removes only the settings " +
			"this resource manages (`mtu`, `comment`, addresses and gateways) and leaves the interface in place.\n\n" +
			"~> PVE resets an interface to `manual` addressing when it is updated without an address. To avoid silently " +
			"dropping an address, creating this resource fails when the interface already carries an address or gateway " +
			"that the configuration does not set.",
		Attributes: map[string]schema.Attribute{
			"id": attribute.ResourceID("A unique identifier with format `<node name>:<iface>`."),
			"node_name": schema.StringAttribute{
				Description: "The name of the node.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of an existing physical network interface, e.g. `eno1` or `nic0`.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"address": schema.StringAttribute{
				Description: "The interface IPv4/CIDR address.",
				CustomType:  customtypes.IPCIDRType{},
				Optional:    true,
			},
			"gateway": schema.StringAttribute{
				Description: "Default gateway address.",
				CustomType:  customtypes.IPAddrType{},
				Optional:    true,
			},
			"address6": schema.StringAttribute{
				Description: "The interface IPv6/CIDR address.",
				CustomType:  customtypes.IPCIDRType{},
				Optional:    true,
			},
			"gateway6": schema.StringAttribute{
				Description: "Default IPv6 gateway address.",
				CustomType:  customtypes.IPAddrType{},
				Optional:    true,
			},
			"autostart": schema.BoolAttribute{
				Description: "Automatically start interface on boot. When not set, the current setting of the interface is kept. " +
					"Not reverted when the resource is destroyed.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"mtu": schema.Int64Attribute{
				Description: "The interface MTU.",
				Optional:    true,
			},
			"comment": schema.StringAttribute{
				Description: "Comment for the interface.",
				Optional:    true,
			},
			"timeout_reload": schema.Int64Attribute{
				Description: "Timeout for network reload operations in seconds (defaults to `100`).",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(int64(nodes.NetworkReloadTimeout.Seconds())),
				Validators: []validator.Int64{
					int64validator.AtLeast(5),
				},
			},
			"reload": schema.BoolAttribute{
				Description: "Whether to reload the node network configuration after this interface is created, " +
					"updated or deleted (defaults to `true`). When `false`, the change is only staged on the node " +
					"and takes effect on the next reload. Any reload on the node, including one triggered by " +
					"another resource, applies all staged changes.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
		},
	}
}

func (r *linuxEthResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(config.Resource)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected config.Resource, got: %T", req.ProviderData),
		)

		return
	}

	r.client = cfg.Client
}

// find returns the eth interface with the given name, or nil when the node has no such interface or it is not of type eth.
func (r *linuxEthResource) find(
	ctx context.Context, nodeName, name string, diags *diag.Diagnostics,
) *nodes.NetworkInterfaceListResponseData {
	ifaces, err := r.client.Node(nodeName).ListNetworkInterfaces(ctx)
	if err != nil {
		diags.AddError(fmt.Sprintf("Unable to List Network Interfaces of Node %q", nodeName), err.Error())
		return nil
	}

	for _, iface := range ifaces {
		if iface.Iface == name && iface.Type == linuxEthType {
			return iface
		}
	}

	return nil
}

func (r *linuxEthResource) reload(ctx context.Context, model *linuxEthResourceModel, diags *diag.Diagnostics) {
	if !model.Reload.IsNull() && !model.Reload.ValueBool() {
		return
	}

	nodeName := model.NodeName.ValueString()

	reloadCtx, cancel := context.WithTimeout(ctx, time.Duration(model.Timeout.ValueInt64())*time.Second)
	defer cancel()

	if err := r.client.Node(nodeName).ReloadNetworkConfiguration(reloadCtx); err != nil {
		diags.AddError(fmt.Sprintf("Unable to Reload Network Configuration of Node %q", nodeName), err.Error())
	}
}

// Create adopts an existing physical interface. The interface is already part of the node's network configuration,
// so it is configured with an update call: PVE rejects a create call for it with "interface already exists".
func (r *linuxEthResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan linuxEthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	nodeName := plan.NodeName.ValueString()
	name := plan.Name.ValueString()

	current := r.find(ctx, nodeName, name, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if current == nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to Create Linux Eth Interface %q", name),
			fmt.Sprintf("Node %q has no physical (eth type) network interface named %q", nodeName, name),
		)

		return
	}

	// an update without an address resets the interface to manual addressing
	for _, unmanaged := range []struct {
		attr    string
		live    *string
		planned bool
	}{
		{"address", current.CIDR, !plan.Address.IsNull()},
		{"gateway", current.Gateway, !plan.Gateway.IsNull()},
		{"address6", current.CIDR6, !plan.Address6.IsNull()},
		{"gateway6", current.Gateway6, !plan.Gateway6.IsNull()},
	} {
		if unmanaged.live != nil && *unmanaged.live != "" && !unmanaged.planned {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Unable to Create Linux Eth Interface %q", name),
				fmt.Sprintf("The interface already has %s %q on node %q, which this resource would remove. "+
					"Set `%s` in the configuration to keep it, or remove it from the interface first.",
					unmanaged.attr, *unmanaged.live, nodeName, unmanaged.attr),
			)
		}
	}

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Node(nodeName).UpdateNetworkInterface(ctx, name, plan.exportToNetworkInterfaceCreateUpdateBody())
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Create Linux Eth Interface %q", name), err.Error())
		return
	}

	plan.ID = types.StringValue(nodeName + ":" + name)

	if !r.readInto(ctx, &plan, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	r.reload(ctx, &plan, &resp.Diagnostics)
}

// readInto refreshes the model from the node and reports whether the interface was found.
func (r *linuxEthResource) readInto(ctx context.Context, model *linuxEthResourceModel, diags *diag.Diagnostics) bool {
	name := model.Name.ValueString()

	iface := r.find(ctx, model.NodeName.ValueString(), name, diags)
	if diags.HasError() {
		return false
	}

	if iface == nil {
		diags.AddError(
			fmt.Sprintf("Unable to Read Linux Eth Interface %q", name),
			fmt.Sprintf("Node %q has no physical (eth type) network interface named %q", model.NodeName.ValueString(), name),
		)

		return false
	}

	model.importFromNetworkInterfaceList(iface)

	return true
}

func (r *linuxEthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state linuxEthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	iface := r.find(ctx, state.NodeName.ValueString(), state.Name.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if iface == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.importFromNetworkInterfaceList(iface)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *linuxEthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state linuxEthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	body := plan.exportToNetworkInterfaceCreateUpdateBody()

	attribute.CheckDelete(plan.Address, state.Address, &body.Delete, "cidr")
	attribute.CheckDelete(plan.Address6, state.Address6, &body.Delete, "cidr6")
	attribute.CheckDelete(plan.MTU, state.MTU, &body.Delete, "mtu")
	attribute.CheckDelete(plan.Gateway, state.Gateway, &body.Delete, "gateway")
	attribute.CheckDelete(plan.Gateway6, state.Gateway6, &body.Delete, "gateway6")
	attribute.CheckDelete(plan.Comment, state.Comment, &body.Delete, "comments")

	err := r.client.Node(plan.NodeName.ValueString()).UpdateNetworkInterface(ctx, name, body)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Update Linux Eth Interface %q", name), err.Error())
		return
	}

	if !r.readInto(ctx, &plan, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	r.reload(ctx, &plan, &resp.Diagnostics)
}

// Delete removes the settings this resource manages. The interface stays in the node's network configuration:
// deleting it through the API would also discard options PVE does not expose, e.g. hand-written `post-up` lines.
func (r *linuxEthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state linuxEthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	nodeName := state.NodeName.ValueString()
	name := state.Name.ValueString()

	iface := r.find(ctx, nodeName, name, &resp.Diagnostics)
	if resp.Diagnostics.HasError() || iface == nil {
		return
	}

	body := &nodes.NetworkInterfaceCreateUpdateRequestBody{Iface: name, Type: linuxEthType}

	for _, managed := range []struct {
		live    *string
		apiName string
	}{
		{iface.CIDR, "cidr"},
		{iface.CIDR6, "cidr6"},
		{iface.Gateway, "gateway"},
		{iface.Gateway6, "gateway6"},
		{iface.MTU, "mtu"},
		{iface.Comments, "comments"},
	} {
		if managed.live != nil {
			body.Delete = append(body.Delete, managed.apiName)
		}
	}

	if len(body.Delete) == 0 {
		return
	}

	err := r.client.Node(nodeName).UpdateNetworkInterface(ctx, name, body)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Delete Linux Eth Interface %q", name), err.Error())
		return
	}

	r.reload(ctx, &state, &resp.Diagnostics)
}

func (r *linuxEthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nodeName, name, ok := strings.Cut(req.ID, ":")
	if !ok || nodeName == "" || name == "" || strings.Contains(name, ":") {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: node_name:iface. Got: %q", req.ID),
		)

		return
	}

	state := linuxEthResourceModel{
		ID:       types.StringValue(req.ID),
		NodeName: types.StringValue(nodeName),
		Name:     types.StringValue(name),
		Timeout:  types.Int64Value(int64(nodes.NetworkReloadTimeout.Seconds())),
		Reload:   types.BoolValue(true),
	}

	if !r.readInto(ctx, &state, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
