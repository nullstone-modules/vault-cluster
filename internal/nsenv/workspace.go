package nsenv

import (
	"context"
	"fmt"
	"strings"

	api "gopkg.in/nullstone-io/go-api-client.v0"
	"gopkg.in/nullstone-io/go-api-client.v0/find"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

// ClusterContract is every Vault cluster module on any cloud.
var ClusterContract = types.ModuleContractName{Category: "datastore", Provider: "*", Platform: "vault", Subplatform: "*"}

type Workspace struct {
	Module   string
	Contract types.ModuleContractName
	Outputs  map[string]any
}

type Resolver interface {
	Workspace(ctx context.Context, stack, env, block string) (Workspace, error)
}

type APIResolver struct {
	Config api.Config
}

func (r APIResolver) Workspace(ctx context.Context, stack, env, block string) (Workspace, error) {
	sbe, err := find.StackBlockEnvByName(ctx, r.Config, stack, block, env)
	if err != nil {
		return Workspace{}, fmt.Errorf("find %s/%s/%s: %w", stack, env, block, err)
	}
	client := api.Client{Config: r.Config}
	sid, bid, eid := sbe.Stack.Id, sbe.Block.Id, sbe.Env.Id

	cfg, err := client.WorkspaceConfigs().GetCurrent(ctx, sid, bid, eid)
	if err != nil {
		return Workspace{}, err
	}
	source := sbe.Block.ModuleSource
	if cfg != nil && cfg.Source != "" {
		source = cfg.Source
	}
	modOrg, modName, err := splitSource(source)
	if err != nil {
		return Workspace{}, err
	}
	mod, err := client.Modules().Get(ctx, modOrg, modName)
	if err != nil {
		return Workspace{}, err
	}
	if mod == nil {
		return Workspace{}, fmt.Errorf("module %s not found", source)
	}

	ws, err := client.Workspaces().Get(ctx, sid, bid, eid)
	if err != nil {
		return Workspace{}, err
	}
	if ws == nil {
		return Workspace{}, fmt.Errorf("workspace %s/%s/%s not found", stack, env, block)
	}
	outs, err := client.WorkspaceOutputs().GetCurrent(ctx, sid, ws.Uid, false)
	if err != nil {
		return Workspace{}, err
	}
	values := map[string]any{}
	for k, o := range outs {
		if !o.Sensitive && !o.Redacted {
			values[k] = o.Value
		}
	}
	return Workspace{
		Module: source,
		Contract: types.ModuleContractName{
			Category:    string(mod.Category),
			Subcategory: string(mod.Subcategory),
			Provider:    strings.Join(mod.ProviderTypes, ","),
			Platform:    mod.Platform,
			Subplatform: mod.Subplatform,
		},
		Outputs: values,
	}, nil
}

// splitSource accepts org/name with an optional registry host.
func splitSource(source string) (string, string, error) {
	parts := strings.Split(strings.Trim(source, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unrecognized module source %q", source)
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}

func CheckContract(ws Workspace) error {
	if !ClusterContract.Match(ws.Contract) {
		return fmt.Errorf("%s (%s) is not a Vault cluster; want a %s workspace", ws.Module, ws.Contract, ClusterContract)
	}
	return nil
}
