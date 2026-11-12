/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package prefix_list

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/bpg/terraform-provider-proxmox/fwprovider/config"
	"github.com/bpg/terraform-provider-proxmox/fwprovider/validators"
	"github.com/bpg/terraform-provider-proxmox/proxmox/api"
	"github.com/bpg/terraform-provider-proxmox/proxmox/cluster/sdn/prefix_lists"
)

var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithConfigure   = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// Resource manages an SDN prefix list.
type Resource struct {
	client *prefix_lists.Client
}

// NewResource creates the proxmox_sdn_prefix_list resource.
func NewResource() resource.Resource {
	return &Resource{}
}

func (r *Resource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "proxmox_sdn_prefix_list"
}

func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.client = cfg.Client.Cluster().SDNPrefixLists()
}

func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Proxmox VE SDN prefix list.",
		MarkdownDescription: "Manages a Proxmox VE SDN prefix list. Prefix lists can be referenced by other SDN objects, " +
			"e.g. as the `route_filter` of a fabric. Changes are staged in the pending SDN configuration " +
			"and take effect when the SDN configuration is applied (see `proxmox_sdn_applier`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the SDN prefix list.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"entries": schema.ListNestedAttribute{
				Description: "The entries of the prefix list, in evaluation order.",
				Optional:    true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"seq": schema.Int64Attribute{
							Description: "The sequence number of the entry.",
							Optional:    true,
							Computed:    true,
							Validators: []validator.Int64{
								int64validator.Between(1, 4294967295),
							},
						},
						"action": schema.StringAttribute{
							Description:         "The action of the entry.",
							MarkdownDescription: "The action of the entry. Must be `permit` or `deny`.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("permit", "deny"),
							},
						},
						"prefix": schema.StringAttribute{
							Description: "The IPv4 or IPv6 prefix in CIDR notation.",
							Required:    true,
							Validators: []validator.String{
								validators.NewParseValidator(netip.ParsePrefix, "must be a valid IPv4 or IPv6 prefix in CIDR notation"),
							},
						},
						"ge": schema.Int64Attribute{
							Description: "Match prefixes with a length greater than or equal to this value.",
							Optional:    true,
							Validators: []validator.Int64{
								int64validator.Between(0, 128),
							},
						},
						"le": schema.Int64Attribute{
							Description: "Match prefixes with a length less than or equal to this value.",
							MarkdownDescription: "Match prefixes with a length less than or equal to this value. " +
								"Without `ge` or `le` an entry matches only the exact prefix and length.",
							Optional: true,
							Validators: []validator.Int64{
								int64validator.Between(0, 128),
							},
						},
					},
				},
			},
		},
	}
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()

	err := r.client.CreatePrefixList(ctx, &prefix_lists.PrefixList{ID: id, Entries: plan.entriesToAPI()})
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Create SDN Prefix List %q", id), err.Error())
		return
	}

	r.readAndSetState(ctx, id, &resp.State, &resp.Diagnostics)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	data, err := r.client.GetPrefixList(ctx, id)
	if err != nil {
		if errors.Is(err, api.ErrResourceDoesNotExist) {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Read SDN Prefix List %q", id), err.Error())

		return
	}

	readModel := &model{}
	readModel.fromAPI(data, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, readModel)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()

	update := &prefix_lists.PrefixListUpdate{Entries: plan.entriesToAPI()}
	if len(update.Entries) == 0 {
		update.Delete = []string{"entries"}
	}

	err := r.client.UpdatePrefixList(ctx, id, update)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Update SDN Prefix List %q", id), err.Error())
		return
	}

	r.readAndSetState(ctx, id, &resp.State, &resp.Diagnostics)
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	err := r.client.DeletePrefixList(ctx, id)
	if err != nil && !errors.Is(err, api.ErrResourceDoesNotExist) {
		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Delete SDN Prefix List %q", id), err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	_, err := r.client.GetPrefixList(ctx, req.ID)
	if err != nil {
		if errors.Is(err, api.ErrResourceDoesNotExist) {
			resp.Diagnostics.AddError(
				"SDN Prefix List Not Found",
				fmt.Sprintf("SDN prefix list with ID %q was not found", req.ID),
			)

			return
		}

		resp.Diagnostics.AddError(fmt.Sprintf("Unable to Import SDN Prefix List %q", req.ID), err.Error())

		return
	}

	r.readAndSetState(ctx, req.ID, &resp.State, &resp.Diagnostics)
}

func (r *Resource) readAndSetState(ctx context.Context, id string, state *tfsdk.State, diags *diag.Diagnostics) {
	data, err := r.client.GetPrefixList(ctx, id)
	if err != nil {
		diags.AddError(fmt.Sprintf("Unable to Read SDN Prefix List %q", id), err.Error())
		return
	}

	readModel := &model{}
	readModel.fromAPI(data, diags)

	if diags.HasError() {
		return
	}

	diags.Append(state.Set(ctx, readModel)...)
}
