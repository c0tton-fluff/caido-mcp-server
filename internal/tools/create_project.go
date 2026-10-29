package tools

import (
	"context"
	"fmt"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// createProjectMutation is hand-written because sdk-go's CreateProject
// selects only error.__typename, so a refusal surfaced as "returned nil".
// The common refusal is PermissionDeniedUserError/ENTITLEMENT: the Caido
// plan's project limit, which applies to temporary projects too.
const createProjectMutation = `mutation CreateProject($input: CreateProjectInput!) {
  createProject(input: $input) {
    project { id name }
    error {
      __typename
      ... on UserError { code }
      ... on PermissionDeniedUserError { permissionReason: reason }
      ... on CloudUserError { cloudReason: reason }
    }
  }
}`

type createProjectResp struct {
	CreateProject struct {
		Project *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"project"`
		Error *struct {
			Typename         string `json:"__typename"`
			Code             string `json:"code"`
			PermissionReason string `json:"permissionReason"`
			CloudReason      string `json:"cloudReason"`
		} `json:"error"`
	} `json:"createProject"`
}

type CreateProjectInput struct {
	Name string `json:"name" jsonschema:"required,Project name"`
}

type CreateProjectOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createProjectHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, CreateProjectInput) (*mcp.CallToolResult, CreateProjectOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input CreateProjectInput,
	) (*mcp.CallToolResult, CreateProjectOutput, error) {
		if input.Name == "" {
			return nil, CreateProjectOutput{}, fmt.Errorf(
				"project name is required",
			)
		}

		var data createProjectResp
		if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
			OpName: "CreateProject",
			Query:  createProjectMutation,
			Variables: map[string]any{"input": map[string]any{
				"name": input.Name, "temporary": false,
			}},
		}, &gql.Response{Data: &data}); err != nil {
			return nil, CreateProjectOutput{}, fmt.Errorf("create project: %w", err)
		}

		payload := data.CreateProject
		if e := payload.Error; e != nil {
			msg := e.Typename
			reason := e.PermissionReason + e.CloudReason
			if reason != "" {
				msg += " (" + reason + ")"
			}
			if reason == "ENTITLEMENT" {
				msg += ": the Caido plan's project limit is reached; " +
					"delete a project or upgrade the plan"
			}
			return nil, CreateProjectOutput{}, fmt.Errorf("create project: %s", msg)
		}
		project := payload.Project
		if project == nil {
			return nil, CreateProjectOutput{}, fmt.Errorf("create project returned no project")
		}

		return nil, CreateProjectOutput{
			ID:   project.ID,
			Name: project.Name,
		}, nil
	}
}

func RegisterCreateProjectTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_create_project",
		Title:       "Create Project",
		Description: `Create a new project with the specified name.`,
		Annotations: writeAnn(false, false, false),
	}, createProjectHandler(client))
}
