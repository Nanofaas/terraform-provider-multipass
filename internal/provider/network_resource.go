package provider

import (
	"context"
	"errors"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/todoroff/terraform-provider-multipass/internal/hostnetwork"
	"time"
)

type networkClient interface {
	Create(context.Context, string, string) (*hostnetwork.Bridge, error)
	Read(context.Context, string) (*hostnetwork.Bridge, error)
	Delete(context.Context, string) error
}
type networkResource struct {
	client  networkClient
	timeout time.Duration
}
type managedNetworkModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
	Active  types.Bool   `tfsdk:"active"`
}

func NewNetworkResource() resource.Resource { return &networkResource{} }
func (r *networkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}
func (r *networkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Persistent private Linux bridge managed with NetworkManager over SSH. This is not a VM isolation boundary.", Attributes: map[string]schema.Attribute{
		"id":      schema.StringAttribute{Computed: true, Description: "Owned NetworkManager connection UUID.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"name":    schema.StringAttribute{Required: true, Description: "Linux bridge interface name (1–15 letters, digits, underscores or hyphens, starting with a letter).", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"address": schema.StringAttribute{Required: true, Description: "Private IPv4 host address/prefix on the bridge, for example 10.60.0.1/24. Does not configure NAT or DHCP.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"active":  schema.BoolAttribute{Computed: true, Description: "Whether the bridge is up with the configured address.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
	}}
}
func (r *networkResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m managedNetworkModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Name.IsUnknown() || m.Address.IsUnknown() {
		return
	}
	if err := hostnetwork.Validate(m.Name.ValueString(), m.Address.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid private bridge", err.Error())
	}
}
func (r *networkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data := req.ProviderData.(providerData)
	if data.ssh == nil {
		resp.Diagnostics.AddError("SSH required", "multipass_network requires ssh_host; local workstation networking is never modified.")
		return
	}
	r.client = hostnetwork.New(data.ssh)
	r.timeout = data.commandTimeout
}
func (r *networkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m managedNetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Network client unavailable", "Configure ssh_host first.")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	found, err := r.client.Create(ctx, m.Name.ValueString(), m.Address.ValueString())
	if found != nil {
		m.set(found)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to create private bridge", err.Error())
	}
}
func (r *networkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m managedNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Network client unavailable", "Configure ssh_host first.")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	found, err := r.client.Read(ctx, m.ID.ValueString())
	if errors.Is(err, hostnetwork.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read private bridge", err.Error())
		return
	}
	m.set(found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *networkResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Bridge replacement required", "Changes to private bridge configuration require replacement.")
}
func (r *networkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m managedNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Network client unavailable", "Configure ssh_host first.")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	if err := r.client.Delete(ctx, m.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete private bridge", err.Error())
	}
}
func (r *networkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Network client unavailable", "Configure ssh_host first.")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	found, err := r.client.Read(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import owned bridge", err.Error())
		return
	}
	var m managedNetworkModel
	m.set(found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *networkResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var m managedNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Active.IsNull() && !m.Active.IsUnknown() && !m.Active.ValueBool() {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("active"))
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("active"), types.BoolUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), types.StringUnknown())...)
	}
}
func (m *managedNetworkModel) set(b *hostnetwork.Bridge) {
	m.ID = types.StringValue(b.ID)
	m.Name = types.StringValue(b.Name)
	m.Address = types.StringValue(b.Address)
	m.Active = types.BoolValue(b.Active)
}
