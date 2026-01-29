package caido

import (
	"context"
	"fmt"
	"net/http"

	"github.com/machinebox/graphql"
)

// Client is a GraphQL client for Caido
type Client struct {
	client   *graphql.Client
	endpoint string
	token    string
}

// NewClient creates a new Caido GraphQL client
func NewClient(endpoint string) *Client {
	graphqlEndpoint := endpoint + "/graphql"
	return &Client{
		client:   graphql.NewClient(graphqlEndpoint),
		endpoint: endpoint,
	}
}

// SetToken sets the authentication token
func (c *Client) SetToken(token string) {
	c.token = token
}

// doRequest executes a GraphQL request with authentication
func (c *Client) doRequest(ctx context.Context, req *graphql.Request, resp interface{}) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.client.Run(ctx, req, resp)
}

// ListRequestsOptions contains options for listing requests
type ListRequestsOptions struct {
	First  int
	After  string
	Filter string // HTTPQL filter
}

// ListRequestsResult is the response from listing requests
type ListRequestsResult struct {
	Requests struct {
		Edges    []RequestEdge `json:"edges"`
		PageInfo PageInfo      `json:"pageInfo"`
	} `json:"requests"`
}

// ListRequests fetches a list of proxied requests
func (c *Client) ListRequests(ctx context.Context, opts ListRequestsOptions) (*ListRequestsResult, error) {
	req := graphql.NewRequest(RequestsQuery)

	if opts.First > 0 {
		req.Var("first", opts.First)
	} else {
		req.Var("first", 20) // default
	}

	if opts.After != "" {
		req.Var("after", opts.After)
	}

	if opts.Filter != "" {
		req.Var("filter", opts.Filter)
	}

	var resp ListRequestsResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to list requests: %w", err)
	}

	return &resp, nil
}

// GetRequestResult is the response from getting a single request
type GetRequestResult struct {
	Request *Request `json:"request"`
}

// GetRequest fetches a single request by ID
func (c *Client) GetRequest(ctx context.Context, id string) (*Request, error) {
	req := graphql.NewRequest(RequestQuery)
	req.Var("id", id)

	var resp GetRequestResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	if resp.Request == nil {
		return nil, fmt.Errorf("request not found: %s", id)
	}

	return resp.Request, nil
}

// StartAuthenticationFlowResult is the response from starting auth flow
type StartAuthenticationFlowResult struct {
	StartAuthenticationFlow struct {
		Request *AuthenticationRequest `json:"request"`
		Error   *struct {
			Typename string `json:"__typename"`
		} `json:"error"`
	} `json:"startAuthenticationFlow"`
}

// StartAuthenticationFlow initiates the OAuth authentication flow
func (c *Client) StartAuthenticationFlow(ctx context.Context) (*AuthenticationRequest, error) {
	req := graphql.NewRequest(StartAuthenticationFlowMutation)

	var resp StartAuthenticationFlowResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to start authentication flow: %w", err)
	}

	if resp.StartAuthenticationFlow.Error != nil {
		return nil, fmt.Errorf("authentication error: %s", resp.StartAuthenticationFlow.Error.Typename)
	}

	return resp.StartAuthenticationFlow.Request, nil
}

// RefreshTokenResult is the response from refreshing the token
type RefreshTokenResult struct {
	RefreshAuthenticationToken struct {
		Token *AuthenticationToken `json:"token"`
		Error *struct {
			Typename string `json:"__typename"`
		} `json:"error"`
	} `json:"refreshAuthenticationToken"`
}

// RefreshToken refreshes the access token using the refresh token
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*AuthenticationToken, error) {
	req := graphql.NewRequest(RefreshAuthenticationTokenMutation)
	req.Var("refreshToken", refreshToken)

	var resp RefreshTokenResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	if resp.RefreshAuthenticationToken.Error != nil {
		return nil, fmt.Errorf("refresh error: %s", resp.RefreshAuthenticationToken.Error.Typename)
	}

	return resp.RefreshAuthenticationToken.Token, nil
}

// WebSocketEndpoint returns the WebSocket endpoint for subscriptions
func (c *Client) WebSocketEndpoint() string {
	// Convert http(s) to ws(s) and use /ws/graphql path
	endpoint := c.endpoint
	if len(endpoint) > 5 && endpoint[:5] == "https" {
		return "wss" + endpoint[5:] + "/ws/graphql"
	}
	return "ws" + endpoint[4:] + "/ws/graphql"
}

// HTTPClient returns the HTTP client for custom requests
func (c *Client) HTTPClient() *http.Client {
	return http.DefaultClient
}

// ListAutomateSessionsResult is the response from listing automate sessions
type ListAutomateSessionsResult struct {
	AutomateSessions AutomateSessionConnection `json:"automateSessions"`
}

// ListAutomateSessions fetches all Automate sessions
func (c *Client) ListAutomateSessions(ctx context.Context) (*ListAutomateSessionsResult, error) {
	req := graphql.NewRequest(AutomateSessionsQuery)

	var resp ListAutomateSessionsResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to list automate sessions: %w", err)
	}

	return &resp, nil
}

// GetAutomateSessionResult is the response from getting a single automate session
type GetAutomateSessionResult struct {
	AutomateSession *AutomateSession `json:"automateSession"`
}

// GetAutomateSession fetches a single Automate session by ID
func (c *Client) GetAutomateSession(ctx context.Context, id string) (*AutomateSession, error) {
	req := graphql.NewRequest(AutomateSessionQuery)
	req.Var("id", id)

	var resp GetAutomateSessionResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get automate session: %w", err)
	}

	if resp.AutomateSession == nil {
		return nil, fmt.Errorf("automate session not found: %s", id)
	}

	return resp.AutomateSession, nil
}

// GetAutomateEntryOptions contains options for getting an automate entry
type GetAutomateEntryOptions struct {
	First int
	After string
}

// GetAutomateEntryResult is the response from getting a single automate entry
type GetAutomateEntryResult struct {
	AutomateEntry *AutomateEntry `json:"automateEntry"`
}

// GetAutomateEntry fetches a single Automate entry with fuzz results
func (c *Client) GetAutomateEntry(ctx context.Context, id string, opts GetAutomateEntryOptions) (*AutomateEntry, error) {
	req := graphql.NewRequest(AutomateEntryQuery)
	req.Var("id", id)

	if opts.First > 0 {
		req.Var("first", opts.First)
	} else {
		req.Var("first", 10) // Small default to save context
	}

	if opts.After != "" {
		req.Var("after", opts.After)
	}

	var resp GetAutomateEntryResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get automate entry: %w", err)
	}

	if resp.AutomateEntry == nil {
		return nil, fmt.Errorf("automate entry not found: %s", id)
	}

	return resp.AutomateEntry, nil
}

// ListReplaySessionsResult is the response from listing replay sessions
type ListReplaySessionsResult struct {
	ReplaySessions ReplaySessionConnection `json:"replaySessions"`
}

// ListReplaySessions fetches all Replay sessions
func (c *Client) ListReplaySessions(ctx context.Context) (*ListReplaySessionsResult, error) {
	req := graphql.NewRequest(ReplaySessionsQuery)

	var resp ListReplaySessionsResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to list replay sessions: %w", err)
	}

	return &resp, nil
}

// GetReplaySessionResult is the response from getting a replay session
type GetReplaySessionResult struct {
	ReplaySession *ReplaySession `json:"replaySession"`
}

// GetReplaySession fetches a single Replay session
func (c *Client) GetReplaySession(ctx context.Context, id string) (*ReplaySession, error) {
	req := graphql.NewRequest(ReplaySessionQuery)
	req.Var("id", id)

	var resp GetReplaySessionResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get replay session: %w", err)
	}

	if resp.ReplaySession == nil {
		return nil, fmt.Errorf("replay session not found: %s", id)
	}

	return resp.ReplaySession, nil
}

// GetReplayEntryResult is the response from getting a replay entry
type GetReplayEntryResult struct {
	ReplayEntry *ReplayEntry `json:"replayEntry"`
}

// GetReplayEntry fetches a single Replay entry
func (c *Client) GetReplayEntry(ctx context.Context, id string) (*ReplayEntry, error) {
	req := graphql.NewRequest(ReplayEntryQuery)
	req.Var("id", id)

	var resp GetReplayEntryResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get replay entry: %w", err)
	}

	if resp.ReplayEntry == nil {
		return nil, fmt.Errorf("replay entry not found: %s", id)
	}

	return resp.ReplayEntry, nil
}

// StartReplayTaskInput is the input for starting a replay task
type StartReplayTaskInput struct {
	Connection ConnectionInfoInput       `json:"connection"`
	Raw        string                    `json:"raw"` // Base64 encoded request
	Settings   ReplayEntrySettingsInput  `json:"settings"`
}

// ConnectionInfoInput is the input for connection info
type ConnectionInfoInput struct {
	Host  string  `json:"host"`
	Port  int     `json:"port"`
	IsTLS bool    `json:"isTLS"`
	SNI   *string `json:"SNI,omitempty"`
}

// ReplayEntrySettingsInput is the input for replay entry settings
type ReplayEntrySettingsInput struct {
	Placeholders        []PlaceholderInput `json:"placeholders"`
	UpdateContentLength bool               `json:"updateContentLength"`
	ConnectionClose     bool               `json:"connectionClose"`
}

// PlaceholderInput is the input for placeholders
type PlaceholderInput struct {
	InputRange  []int  `json:"inputRange"`
	OutputRange []int  `json:"outputRange"`
	Preprocessors []string `json:"preprocessors"`
}

// StartReplayTaskResult is the response from starting a replay task
type StartReplayTaskResult struct {
	StartReplayTask struct {
		Task *struct {
			ID string `json:"id"`
		} `json:"task"`
		Error *struct {
			Typename string `json:"__typename"`
			Code     string `json:"code,omitempty"`
		} `json:"error"`
	} `json:"startReplayTask"`
}

// StartReplayTask sends a request via Replay
func (c *Client) StartReplayTask(ctx context.Context, sessionID string, input StartReplayTaskInput) (string, error) {
	req := graphql.NewRequest(StartReplayTaskMutation)
	req.Var("sessionId", sessionID)
	req.Var("input", input)

	var resp StartReplayTaskResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return "", fmt.Errorf("failed to start replay task: %w", err)
	}

	if resp.StartReplayTask.Error != nil {
		return "", fmt.Errorf("replay error: %s", resp.StartReplayTask.Error.Typename)
	}

	if resp.StartReplayTask.Task == nil {
		return "", fmt.Errorf("no task returned")
	}

	return resp.StartReplayTask.Task.ID, nil
}

// ListFindingsOptions contains options for listing findings
type ListFindingsOptions struct {
	First  int
	After  string
	Filter string
}

// ListFindingsResult is the response from listing findings
type ListFindingsResult struct {
	Findings FindingConnection `json:"findings"`
}

// ListFindings fetches findings
func (c *Client) ListFindings(ctx context.Context, opts ListFindingsOptions) (*ListFindingsResult, error) {
	req := graphql.NewRequest(FindingsQuery)

	if opts.First > 0 {
		req.Var("first", opts.First)
	} else {
		req.Var("first", 10)
	}

	if opts.After != "" {
		req.Var("after", opts.After)
	}

	if opts.Filter != "" {
		req.Var("filter", opts.Filter)
	}

	var resp ListFindingsResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to list findings: %w", err)
	}

	return &resp, nil
}

// CreateFindingInput is the input for creating a finding
type CreateFindingInput struct {
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	Reporter    string  `json:"reporter"`
	DedupeKey   *string `json:"dedupeKey,omitempty"`
}

// CreateFindingResult is the response from creating a finding
type CreateFindingResult struct {
	CreateFinding struct {
		Finding *Finding `json:"finding"`
		Error   *struct {
			Typename string `json:"__typename"`
		} `json:"error"`
	} `json:"createFinding"`
}

// CreateFinding creates a new finding
func (c *Client) CreateFinding(ctx context.Context, requestID string, input CreateFindingInput) (*Finding, error) {
	req := graphql.NewRequest(CreateFindingMutation)
	req.Var("requestId", requestID)
	req.Var("input", input)

	var resp CreateFindingResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to create finding: %w", err)
	}

	if resp.CreateFinding.Error != nil {
		return nil, fmt.Errorf("create finding error: %s", resp.CreateFinding.Error.Typename)
	}

	return resp.CreateFinding.Finding, nil
}

// SitemapRootEntriesResult is the response from getting root sitemap entries
type SitemapRootEntriesResult struct {
	SitemapRootEntries SitemapEntryConnection `json:"sitemapRootEntries"`
}

// GetSitemapRootEntries fetches root sitemap entries
func (c *Client) GetSitemapRootEntries(ctx context.Context) (*SitemapRootEntriesResult, error) {
	req := graphql.NewRequest(SitemapRootEntriesQuery)

	var resp SitemapRootEntriesResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get sitemap root entries: %w", err)
	}

	return &resp, nil
}

// SitemapDescendantEntriesResult is the response from getting descendant entries
type SitemapDescendantEntriesResult struct {
	SitemapDescendantEntries SitemapEntryConnection `json:"sitemapDescendantEntries"`
}

// GetSitemapDescendantEntries fetches children of a sitemap entry
func (c *Client) GetSitemapDescendantEntries(ctx context.Context, id string) (*SitemapDescendantEntriesResult, error) {
	req := graphql.NewRequest(SitemapDescendantEntriesQuery)
	req.Var("parentId", id)
	req.Var("depth", "DIRECT") // Get immediate children only

	var resp SitemapDescendantEntriesResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get sitemap descendants: %w", err)
	}

	return &resp, nil
}

// ListScopesResult is the response from listing scopes
type ListScopesResult struct {
	Scopes []Scope `json:"scopes"`
}

// ListScopes fetches all scopes
func (c *Client) ListScopes(ctx context.Context) (*ListScopesResult, error) {
	req := graphql.NewRequest(ScopesQuery)

	var resp ListScopesResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to list scopes: %w", err)
	}

	return &resp, nil
}

// CreateScopeInput is the input for creating a scope
type CreateScopeInput struct {
	Name      string   `json:"name"`
	Allowlist []string `json:"allowlist"`
	Denylist  []string `json:"denylist"`
}

// CreateScopeResult is the response from creating a scope
type CreateScopeResult struct {
	CreateScope struct {
		Scope *Scope `json:"scope"`
		Error *struct {
			Typename string `json:"__typename"`
		} `json:"error"`
	} `json:"createScope"`
}

// CreateScope creates a new scope
func (c *Client) CreateScope(ctx context.Context, input CreateScopeInput) (*Scope, error) {
	req := graphql.NewRequest(CreateScopeMutation)
	req.Var("input", input)

	var resp CreateScopeResult
	if err := c.doRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to create scope: %w", err)
	}

	if resp.CreateScope.Error != nil {
		return nil, fmt.Errorf("create scope error: %s", resp.CreateScope.Error.Typename)
	}

	return resp.CreateScope.Scope, nil
}
