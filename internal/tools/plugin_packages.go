package tools

import (
	"context"
	"fmt"
	"strings"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
)

// pluginPackagesQuery lists installed packages with their backend plugins.
// The sdk-go ListPluginPackages operation selects neither the package
// manifestId nor any backend state, and there is no separate backend query, so
// this is a hand-written request. It is validated against the vendored Caido
// schema in the tests.
const pluginPackagesQuery = `query PluginPackagesWithBackends {
	pluginPackages {
		id
		manifestId
		name
		description
		version
		plugins {
			__typename
			id
			enabled
			... on PluginBackend {
				manifestId
				name
				state { running error }
			}
		}
	}
}`

// pluginState is PluginState: whether the backend JS runtime is up.
type pluginState struct {
	Running bool    `json:"running"`
	Error   *string `json:"error"`
}

// pluginNode is one Plugin interface entry (backend or frontend).
type pluginNode struct {
	Typename   string       `json:"__typename"`
	ID         string       `json:"id"`
	Enabled    bool         `json:"enabled"`
	ManifestID string       `json:"manifestId"`
	Name       *string      `json:"name"`
	State      *pluginState `json:"state"`
}

// pluginPackage is one installed PluginPackage.
type pluginPackage struct {
	ID          string       `json:"id"`
	ManifestID  string       `json:"manifestId"`
	Name        *string      `json:"name"`
	Description *string      `json:"description"`
	Version     string       `json:"version"`
	Plugins     []pluginNode `json:"plugins"`
}

// backend returns the package's backend plugin, or nil when it only ships a
// frontend.
func (p pluginPackage) backend() *pluginNode {
	for i := range p.Plugins {
		if p.Plugins[i].Typename == "PluginBackend" {
			return &p.Plugins[i]
		}
	}
	return nil
}

// displayName is the package name, or its manifestId when Caido has no name.
func (p pluginPackage) displayName() string {
	if p.Name != nil && *p.Name != "" {
		return *p.Name
	}
	return p.ManifestID
}

// listPluginPackages fetches every installed plugin package with backend state.
func listPluginPackages(
	ctx context.Context, client *caido.Client,
) ([]pluginPackage, error) {
	var data struct {
		PluginPackages []pluginPackage `json:"pluginPackages"`
	}
	if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
		OpName: "PluginPackagesWithBackends",
		Query:  pluginPackagesQuery,
	}, &gql.Response{Data: &data}); err != nil {
		return nil, fmt.Errorf("list plugin packages: %w", err)
	}
	return data.PluginPackages, nil
}

// pluginTarget is a resolved package and the backend to call.
type pluginTarget struct {
	Package pluginPackage
	Backend pluginNode
}

// resolvePluginTarget matches want against a package manifestId, package
// name, package id or backend plugin id, case-insensitively. Backend
// manifestIds are not matched on their own: several plugins name theirs
// "backend", which would match everything.
func resolvePluginTarget(
	packages []pluginPackage, want string,
) (*pluginTarget, error) {
	name := strings.TrimSpace(want)
	if name == "" {
		return nil, fmt.Errorf("plugin is required")
	}

	var matched *pluginPackage
	for i := range packages {
		p := &packages[i]
		backend := p.backend()
		if strings.EqualFold(p.ManifestID, name) ||
			strings.EqualFold(p.ID, name) ||
			(p.Name != nil && strings.EqualFold(*p.Name, name)) ||
			(backend != nil && strings.EqualFold(backend.ID, name)) {
			if matched != nil {
				return nil, fmt.Errorf(
					"plugin %q matches more than one installed package; "+
						"use its manifestId (%s or %s)",
					want, matched.ManifestID, p.ManifestID,
				)
			}
			matched = p
		}
	}
	if matched == nil {
		return nil, fmt.Errorf(
			"unknown plugin %q; installed plugins: %s",
			want, installedManifestIDs(packages),
		)
	}

	backend := matched.backend()
	if backend == nil {
		return nil, fmt.Errorf(
			"plugin %q (%s) has no backend; only plugins with a "+
				"backend expose functions",
			matched.displayName(), matched.ManifestID,
		)
	}
	if backend.State != nil && !backend.State.Running {
		return nil, pluginNotRunningError(*matched, *backend)
	}

	return &pluginTarget{Package: *matched, Backend: *backend}, nil
}

// installedManifestIDs renders the installed plugins for an unknown-name
// error. The order is Caido's.
func installedManifestIDs(packages []pluginPackage) string {
	ids := make([]string, 0, len(packages))
	for _, p := range packages {
		ids = append(ids, p.ManifestID)
	}
	if len(ids) == 0 {
		return "(none installed)"
	}
	return strings.Join(ids, ", ")
}

// pluginNotRunningError explains the Caido 0.59 failure mode: a backend
// installed while Caido is running has no live JS runtime until a restart, and
// the GraphQL state says so.
func pluginNotRunningError(p pluginPackage, b pluginNode) error {
	msg := fmt.Sprintf(
		"plugin %q backend is not running: restart Caido; plugins installed "+
			"while Caido is running are not callable until then",
		p.ManifestID,
	)
	if b.State != nil && b.State.Error != nil && *b.State.Error != "" {
		msg += fmt.Sprintf(" (backend error: %s)", *b.State.Error)
	}
	return fmt.Errorf("%s", msg)
}
