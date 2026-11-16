package tools

import (
	"context"
	"fmt"
	"strings"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sitemapRootQuery lists root domains. The nested request count is behind
// @include: Caido has no per-entry count field on the connection, only a
// requests connection on each entry, so asking for it costs one count per
// entry (2.9 s for a 6,971 entry subtree against 0.7 s without), and most tree
// walks do not need it.
const sitemapRootQuery = `query SitemapRootEntries($scopeId: ID, $counts: Boolean!) {
	sitemapRootEntries(scopeId: $scopeId) {
		count { value }
		edges {
			node {
				id
				label
				kind
				parentId
				hasDescendants
				requests @include(if: $counts) { count { value } }
			}
		}
	}
}`

// sitemapDescendantQuery lists children of an entry, DIRECT or ALL. Caido
// 0.59.0 accepts no first/after on either connection, so the whole
// connection comes back and a limit is applied here.
const sitemapDescendantQuery = `query SitemapDescendantEntries($parentId: ID!, $depth: SitemapDescendantsDepth!, $counts: Boolean!) {
	sitemapDescendantEntries(parentId: $parentId, depth: $depth) {
		count { value }
		edges {
			node {
				id
				label
				kind
				parentId
				hasDescendants
				requests @include(if: $counts) { count { value } }
				request {
					id
					method
					response { statusCode }
				}
			}
		}
	}
}`

// sitemapConnection is the wire shape of SitemapEntryConnection for the fields
// the queries select. Count is the number of entries in the connection.
type sitemapConnection struct {
	Count sitemapCount `json:"count"`
	Edges []struct {
		Node sitemapNode `json:"node"`
	} `json:"edges"`
}

type sitemapCount struct {
	Value int `json:"value"`
}

type sitemapNode struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	Kind           string  `json:"kind"`
	ParentID       *string `json:"parentId"`
	HasDescendants bool    `json:"hasDescendants"`
	// Requests is absent unless the query included it.
	Requests *struct {
		Count sitemapCount `json:"count"`
	} `json:"requests"`
	Request *struct {
		ID       string `json:"id"`
		Method   string `json:"method"`
		Response *struct {
			StatusCode int `json:"statusCode"`
		} `json:"response"`
	} `json:"request"`
}

// GetSitemapInput is the input for the get_sitemap tool
type GetSitemapInput struct {
	ParentID string `json:"parentId,omitempty" jsonschema:"Parent entry ID to list children of (omit for root domains)"`
	Depth    string `json:"depth,omitempty" jsonschema:"With parentId: direct (default) or all descendants"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Max entries to return (default 0 = every entry Caido returns); Caido has no cursor for this connection, so this truncates locally"`
	// IncludeRequestCounts adds each entry's own captured-request count.
	IncludeRequestCounts bool `json:"includeRequestCounts,omitempty" jsonschema:"Add each entry's captured-request count; it costs one nested count per entry, seconds on a large depth all tree"`
}

// SitemapEntrySummary is a summary of a sitemap entry
type SitemapEntrySummary struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	Kind           string  `json:"kind"`
	HasDescendants bool    `json:"hasDescendants"`
	RequestID      *string `json:"requestId,omitempty"`
	Method         *string `json:"method,omitempty"`
	StatusCode     *int    `json:"statusCode,omitempty"`
	// RequestCount is the number of captured requests attached to this
	// entry, set only with includeRequestCounts. Entries overlap (a request
	// appears under its directory, its path and its query), so these
	// counts do not add up to a host total.
	RequestCount *int `json:"requestCount,omitempty"`
	// CapturedRequests is the exact number of captured requests whose host
	// is this root entry's label, from the request history rather than the
	// sitemap tree. Set on root DOMAIN entries only.
	CapturedRequests *int `json:"capturedRequests,omitempty"`
}

// GetSitemapOutput is the output of the get_sitemap tool
type GetSitemapOutput struct {
	Entries []SitemapEntrySummary `json:"entries"`
	// TotalEntries is the connection count before limit.
	TotalEntries int `json:"totalEntries"`
	// Truncated reports that limit dropped entries.
	Truncated bool `json:"truncated,omitempty"`
}

// getSitemapHandler creates the handler function
func getSitemapHandler(
	client *caido.Client,
) func(context.Context, *mcp.CallToolRequest, GetSitemapInput) (*mcp.CallToolResult, GetSitemapOutput, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetSitemapInput,
	) (*mcp.CallToolResult, GetSitemapOutput, error) {
		depth, err := sitemapDepth(input.Depth, input.ParentID)
		if err != nil {
			return nil, GetSitemapOutput{}, err
		}

		var conn sitemapConnection
		if input.ParentID == "" {
			var data struct {
				SitemapRootEntries sitemapConnection `json:"sitemapRootEntries"`
			}
			if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
				OpName: "SitemapRootEntries",
				Query:  sitemapRootQuery,
				Variables: map[string]any{
					"scopeId": nil,
					"counts":  input.IncludeRequestCounts,
				},
			}, &gql.Response{Data: &data}); err != nil {
				return nil, GetSitemapOutput{}, fmt.Errorf(
					"list sitemap roots: %w", err,
				)
			}
			conn = data.SitemapRootEntries
		} else {
			var data struct {
				SitemapDescendantEntries sitemapConnection `json:"sitemapDescendantEntries"`
			}
			if err := client.GraphQL.MakeRequest(ctx, &gql.Request{
				OpName: "SitemapDescendantEntries",
				Query:  sitemapDescendantQuery,
				Variables: map[string]any{
					"parentId": input.ParentID,
					"depth":    depth,
					"counts":   input.IncludeRequestCounts,
				},
			}, &gql.Response{Data: &data}); err != nil {
				return nil, GetSitemapOutput{}, fmt.Errorf(
					"list sitemap children of %s: %w", input.ParentID, err,
				)
			}
			conn = data.SitemapDescendantEntries
		}

		output := GetSitemapOutput{
			Entries:      make([]SitemapEntrySummary, 0, len(conn.Edges)),
			TotalEntries: conn.Count.Value,
		}
		for _, edge := range conn.Edges {
			output.Entries = append(
				output.Entries, summarizeSitemapNode(edge.Node),
			)
		}

		if input.ParentID == "" {
			for i := range output.Entries {
				entry := &output.Entries[i]
				if entry.Kind != "DOMAIN" {
					continue
				}
				total, err := hostRequestTotal(ctx, client, entry.Label)
				if err != nil {
					return nil, GetSitemapOutput{}, err
				}
				entry.CapturedRequests = &total
			}
		}

		if input.Limit > 0 && len(output.Entries) > input.Limit {
			output.Entries = output.Entries[:input.Limit]
			output.Truncated = true
		}

		return nil, output, nil
	}
}

// sitemapDepth validates the depth option against the parent ID.
func sitemapDepth(depth, parentID string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(depth)) {
	case "":
		return "DIRECT", nil
	case "direct":
		if parentID == "" {
			return "", fmt.Errorf("depth requires parentId")
		}
		return "DIRECT", nil
	case "all":
		if parentID == "" {
			return "", fmt.Errorf("depth requires parentId")
		}
		return "ALL", nil
	default:
		return "", fmt.Errorf(
			"depth must be one of [direct all], got %q", depth,
		)
	}
}

// summarizeSitemapNode converts a wire entry into the tool row.
func summarizeSitemapNode(node sitemapNode) SitemapEntrySummary {
	summary := SitemapEntrySummary{
		ID:             node.ID,
		Label:          node.Label,
		Kind:           node.Kind,
		HasDescendants: node.HasDescendants,
	}
	if node.Requests != nil {
		count := node.Requests.Count.Value
		summary.RequestCount = &count
	}
	if node.Request != nil {
		id := node.Request.ID
		method := node.Request.Method
		summary.RequestID = &id
		summary.Method = &method
		if node.Request.Response != nil {
			status := node.Request.Response.StatusCode
			summary.StatusCode = &status
		}
	}
	return summary
}

// hostRequestTotal counts captured requests whose host is host. The sitemap
// tree cannot answer this: one request is listed under its directory, its
// path and its query, so summing entry counts overcounts, and a host entry's
// own requests connection covers only part of its traffic (6,718 of 7,875 on
// the corpus this was measured against).
func hostRequestTotal(
	ctx context.Context, client *caido.Client, host string,
) (int, error) {
	if strings.ContainsAny(host, `"\`) {
		return 0, fmt.Errorf(
			"cannot count requests for sitemap label %q", host,
		)
	}
	one := 1
	filter := fmt.Sprintf(`req.host.eq:"%s"`, host)
	resp, err := client.Requests.List(ctx, &caido.ListRequestsOptions{
		First:  &one,
		Filter: &filter,
	})
	if err != nil {
		return 0, fmt.Errorf("count requests for host %s: %w", host, err)
	}
	return resp.Requests.Count.Value, nil
}

// RegisterGetSitemapTool registers the tool with the MCP server
func RegisterGetSitemapTool(
	server *mcp.Server, client *caido.Client,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "caido_get_sitemap",
		Title:       "Get Sitemap",
		Description: `Browse the sitemap tree. No params: root domains, each with capturedRequests, the exact count of captured requests for that host. parentId: its children, depth:"all" for the whole subtree. Rows carry id/label/kind/hasDescendants, plus requestId/method/statusCode for request entries. includeRequestCounts adds each entry's own request count (slow on a large depth:"all" tree); limit truncates locally because Caido exposes no cursor here.`,
		InputSchema: schemaFor[GetSitemapInput](func(s *jsonschema.Schema) {
			if p := prop(s, "depth"); p != nil {
				p.Enum = []any{"direct", "all"}
			}
		}),
		Annotations: readOnlyAnn(),
	}, getSitemapHandler(client))
}
