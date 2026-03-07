package tools

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListProjectsInput is the input for the list_projects tool
type ListProjectsInput struct{}

// ProjectSummary is a summary of a project
type ProjectSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Version   string `json:"version"`
	IsCurrent bool   `json:"isCurrent"`
}

// ListProjectsOutput is the output of the list_projects tool
type ListProjectsOutput struct {
	Projects []ProjectSummary `json:"projects"`
}

// listProjectsHandler creates the handler function
func listProjectsHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, ListProjectsInput) (*mcp.CallToolResult, ListProjectsOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input ListProjectsInput,
	) (*mcp.CallToolResult, ListProjectsOutput, error) {
		listResp, err := client.Projects.List(ctx)
		if err != nil {
			return nil, ListProjectsOutput{}, err
		}

		currentResp, err := client.Projects.GetCurrent(ctx)
		if err != nil {
			return nil, ListProjectsOutput{}, err
		}

		var currentID string
		if currentResp.CurrentProject != nil &&
			currentResp.CurrentProject.Project != nil {
			currentID = currentResp.CurrentProject.Project.Id
		}

		output := ListProjectsOutput{
			Projects: make(
				[]ProjectSummary, 0,
				len(listResp.Projects),
			),
		}

		for _, p := range listResp.Projects {
			output.Projects = append(
				output.Projects, ProjectSummary{
					ID:        p.Id,
					Name:      p.Name,
					Status:    string(p.Status),
					Version:   p.Version,
					IsCurrent: p.Id == currentID,
				},
			)
		}

		return nil, output, nil
	}
}

// RegisterListProjectsTool registers the tool
func RegisterListProjectsTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "caido_list_projects",
		Description: `List projects. Returns id/name/status/version. ` +
			`Current project marked with isCurrent.`,
	}, listProjectsHandler(client))
}
