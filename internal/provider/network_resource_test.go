package provider

import (
	"context"
	"errors"
	"github.com/Nanofaas/terraform-provider-multipass/internal/hostnetwork"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"testing"
	"time"
)

type networkFake struct {
	bridge           *hostnetwork.Bridge
	err              error
	creates, deletes int
}

func (f *networkFake) Create(context.Context, string, string) (*hostnetwork.Bridge, error) {
	f.creates++
	return f.bridge, f.err
}
func (f *networkFake) Read(context.Context, string) (*hostnetwork.Bridge, error) {
	return f.bridge, f.err
}
func (f *networkFake) Delete(context.Context, string) error { f.deletes++; return f.err }
func networkState(r *networkResource) tfsdk.State {
	s := resourceSchema(r)
	return tfsdk.State{Schema: s, Raw: resourceValue(s, map[string]any{"id": "afc40530-1111-4111-8111-111111111111", "name": "nf-test", "address": "10.60.0.1/24", "active": true})}
}
func TestNetworkRequiresSSHAndValidation(t *testing.T) {
	r := NewNetworkResource().(*networkResource)
	var resp resource.ConfigureResponse
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: providerData{}}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("local network mutation allowed")
	}
	state := networkState(r)
	state.Raw = resourceValue(resourceSchema(r), map[string]any{"name": "bad;name", "address": "8.8.8.8/24"})
	var vr resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: state.Schema, Raw: state.Raw}}, &vr)
	if !vr.Diagnostics.HasError() {
		t.Fatal("invalid network accepted")
	}
}
func TestNetworkLifecycle(t *testing.T) {
	f := &networkFake{bridge: &hostnetwork.Bridge{ID: "afc40530-1111-4111-8111-111111111111", Name: "nf-test", Address: "10.60.0.1/24", Active: true}}
	r := &networkResource{client: f, timeout: time.Second}
	state := networkState(r)
	for _, fail := range []bool{false, true} {
		f.err = nil
		if fail {
			f.err = errors.New("activation failed")
		}
		resp := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
		r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}}, &resp)
		if resp.Diagnostics.HasError() != fail {
			t.Fatal(resp.Diagnostics)
		}
		var got managedNetworkModel
		resp.State.Get(context.Background(), &got)
		if got.ID.ValueString() != f.bridge.ID {
			t.Fatal("lost partial create identity")
		}
	}
	f.bridge = nil
	for _, err := range []error{errors.New("SSH unavailable"), hostnetwork.ErrNotFound} {
		f.err = err
		resp := resource.ReadResponse{State: state}
		r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
		if errors.Is(err, hostnetwork.ErrNotFound) {
			if !resp.State.Raw.IsNull() {
				t.Fatal("missing network retained")
			}
		} else {
			if resp.State.Raw.IsNull() || !resp.Diagnostics.HasError() {
				t.Fatal("unavailable host removed state")
			}
		}
	}
	if f.creates != 2 || f.deletes != 0 {
		t.Fatal("refresh mutated host")
	}
}
func TestNetworkReplacementAndImport(t *testing.T) {
	f := &networkFake{bridge: &hostnetwork.Bridge{ID: "afc40530-1111-4111-8111-111111111111", Name: "nf-test", Address: "10.60.0.1/24"}}
	r := &networkResource{client: f, timeout: time.Second}
	state := networkState(r)
	state.SetAttribute(context.Background(), path.Root("active"), types.BoolValue(false))
	plan := tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}
	resp := resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{State: state, Plan: plan}, &resp)
	if len(resp.RequiresReplace) == 0 {
		t.Fatal("inactive bridge did not require repair")
	}
	imported := resource.ImportStateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: f.bridge.ID}, &imported)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	f.err = errors.New("not owned")
	rejected := resource.ImportStateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: f.bridge.ID}, &rejected)
	if !rejected.Diagnostics.HasError() {
		t.Fatal("unowned import allowed")
	}
	f.err = errors.New("attached ports")
	deletion := resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &deletion)
	if !deletion.Diagnostics.HasError() {
		t.Fatal("attached bridge deleted")
	}
}
