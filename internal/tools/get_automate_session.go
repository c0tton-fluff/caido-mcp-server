package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/c0tton-fluff/caido-mcp-server/internal/caido"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetAutomateSessionInput is the input for the get_automate_session tool
type GetAutomateSessionInput struct {
	ID string `json:"id" jsonschema:"required,Automate session ID"`
}

// AutomateEntrySummary is a minimal representation of an Automate entry
type AutomateEntrySummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// GetAutomateSessionOutput is the output of the get_automate_session tool
type GetAutomateSessionOutput struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	CreatedAt       string                 `json:"createdAt"`
	RequestTemplate string                 `json:"requestTemplate"` // Decoded request template
	Entries         []AutomateEntrySummary `json:"entries"`
}

// getAutomateSessionHandler creates the handler function for the get_automate_session tool
func getAutomateSessionHandler(client *caido.Client) func(context.Context, *mcp.CallToolRequest, GetAutomateSessionInput) (*mcp.CallToolResult, GetAutomateSessionOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetAutomateSessionInput) (*mcp.CallToolResult, GetAutomateSessionOutput, error) {
		if input.ID == "" {
			return nil, GetAutomateSessionOutput{}, fmt.Errorf("session ID is required")
		}

		session, err := client.GetAutomateSession(ctx, input.ID)
		if err != nil {
			return nil, GetAutomateSessionOutput{}, err
		}

		// Decode the request template from base64
		requestTemplate := ""
		if session.Raw != "" {
			decoded, err := base64.StdEncoding.DecodeString(session.Raw)
			if err == nil {
				requestTemplate = string(decoded)
			}
		}

		output := GetAutomateSessionOutput{
			ID:              session.ID,
			Name:            session.Name,
			CreatedAt:       session.CreatedAt.Time().Format(time.RFC3339),
			RequestTemplate: requestTemplate,
			Entries:         make([]AutomateEntrySummary, 0, len(session.Entries)),
		}

		for _, e := range session.Entries {
			output.Entries = append(output.Entries, AutomateEntrySummary{
				ID:        e.ID,
				Name:      e.Name,
				CreatedAt: e.CreatedAt.Time().Format(time.RFC3339),
			})
		}

		return nil, output, nil
	}
}

// RegisterGetAutomateSessionTool registers the tool with the MCP server
func RegisterGetAutomateSessionTool(server *mcp.Server, client *caido.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_automate_session",
		Description: `Get fuzzing session details. Returns requestTemplate and list of entry IDs.`,
	}, getAutomateSessionHandler(client))
}
