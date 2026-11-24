package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	gql "github.com/Khan/genqlient/graphql"
	caido "github.com/caido-community/sdk-go"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sitemapRootQuery lists root domains. A root entry is one host on one port
// and scheme, so the same label can appear many times (a local test server on
// thousands of ephemeral ports); metadata tells them apart. The nested
// request count is behind @include: Caido has no per-entry count field on the
// connection, only a requests connection on each entry, so asking for it
// costs one count per entry (2.9 s for a 6,971 entry subtree against 0.7 s
// without), and most tree walks do not need it.
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
				metadata { ... on SitemapEntryMetadataDomain { isTls port } }
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
	// Metadata is selected for root entries only.
	Metadata *struct {
		IsTLS bool `json:"isTls"`
		Port  int  `json:"port"`
	} `json:"metadata"`
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
	// IncludeRequestCounts adds each entry's own captured-request count,
	// and on root domains the count from the request history.
	IncludeRequestCounts bool `json:"includeRequestCounts,omitempty" jsonschema:"Add requestCount per entry, and capturedRequests on root domains. Costs one count per entry: use limit on a large tree"`
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
	// Port and IsTLS identify a root DOMAIN entry together with its label.
	Port  *int `json:"port,omitempty"`
	IsTLS bool `json:"isTls,omitempty"`
	// RequestCount is the number of captured requests attached to this
	// entry, set only with includeRequestCounts. Entries overlap (a request
	// appears under its directory, its path and its query), so these
	// counts do not add up to a host total.
	RequestCount *int `json:"requestCount,omitempty"`
	// CapturedRequests is the number of captured requests for this root
	// entry's host, port and scheme, counted in the request history rather
	// than the sitemap tree. Set on root DOMAIN entries with
	// includeRequestCounts only: it costs one history count per entry.
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

		if input.Limit > 0 && len(output.Entries) > input.Limit {
			output.Entries = output.Entries[:input.Limit]
			output.Truncated = true
		}

		// History counts come after the limit, and only on request: one
		// count per root entry took 103 s on a project with 6,819 of them.
		if input.ParentID == "" && input.IncludeRequestCounts {
			if err := addCapturedRequests(ctx, client, output.Entries); err != nil {
				return nil, GetSitemapOutput{}, err
			}
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
	if node.Metadata != nil {
		port := node.Metadata.Port
		summary.Port = &port
		summary.IsTLS = node.Metadata.IsTLS
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

// sitemapCountConcurrency bounds parallel history counts for one listing.
const sitemapCountConcurrency = 8

// addCapturedRequests fills CapturedRequests on root DOMAIN entries. The
// sitemap tree cannot answer this: one request is listed under its
// directory, its path and its query, so summing entry counts overcounts, and
// a host entry's own requests connection covers only part of its traffic
// (6,718 of 7,875 on the corpus this was measured against).
//
// A count that fails leaves that entry without the field rather than failing
// a listing of thousands of entries; the error is returned only when no count
// succeeded, which means the counts are not available at all.
func addCapturedRequests(
	ctx context.Context, client *caido.Client, entries []SitemapEntrySummary,
) error {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		counted  int
	)
	sem := make(chan struct{}, sitemapCountConcurrency)
	for i := range entries {
		entry := &entries[i]
		if entry.Kind != "DOMAIN" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			total, err := rootRequestTotal(ctx, client, *entry)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			entry.CapturedRequests = &total
			counted++
		}()
	}
	wg.Wait()
	if counted == 0 {
		return firstErr
	}
	return nil
}

// rootRequestFilter is the HTTPQL filter selecting one root entry's
// requests. Host alone is not enough: the same host on another port or
// scheme is a different root entry.
func rootRequestFilter(entry SitemapEntrySummary) (string, error) {
	if strings.ContainsAny(entry.Label, `"\`) {
		return "", fmt.Errorf(
			"cannot count requests for sitemap label %q", entry.Label,
		)
	}
	filter := fmt.Sprintf(`req.host.eq:"%s"`, entry.Label)
	if entry.Port != nil {
		filter += fmt.Sprintf(
			" and req.port.eq:%d and req.tls.eq:%t", *entry.Port, entry.IsTLS,
		)
	}
	return filter, nil
}

// rootRequestTotal counts the captured requests of one root entry.
func rootRequestTotal(
	ctx context.Context, client *caido.Client, entry SitemapEntrySummary,
) (int, error) {
	filter, err := rootRequestFilter(entry)
	if err != nil {
		return 0, err
	}
	one := 1
	resp, err := client.Requests.List(ctx, &caido.ListRequestsOptions{
		First:  &one,
		Filter: &filter,
	})
	if err != nil {
		return 0, fmt.Errorf(
			"count requests for host %s: %w", entry.Label, err,
		)
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
		Description: `Browse the sitemap tree. No params: root domains (one per host, port and scheme). parentId: its children, depth:"all" for the whole subtree. Rows carry id/label/kind/hasDescendants, plus requestId/method/statusCode for request entries. limit truncates locally (Caido has no cursor here). includeRequestCounts adds requestCount per entry and capturedRequests per root domain; it costs one count per entry, so pair it with limit.`,
		InputSchema: schemaFor[GetSitemapInput](func(s *jsonschema.Schema) {
			if p := prop(s, "depth"); p != nil {
				p.Enum = []any{"direct", "all"}
			}
		}),
		Annotations: readOnlyAnn(),
	}, getSitemapHandler(client))
}
