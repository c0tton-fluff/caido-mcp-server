package caido

import (
	"encoding/json"
	"time"
)

// Timestamp is a custom type for handling Unix millisecond timestamps
type Timestamp time.Time

// UnmarshalJSON handles both Unix milliseconds (int) and RFC3339 strings
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	// Try as integer (Unix milliseconds)
	var ms int64
	if err := json.Unmarshal(data, &ms); err == nil {
		*t = Timestamp(time.UnixMilli(ms))
		return nil
	}

	// Try as string (RFC3339)
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		parsed, err := time.Parse(time.RFC3339, str)
		if err != nil {
			return err
		}
		*t = Timestamp(parsed)
		return nil
	}

	return nil
}

// Time returns the underlying time.Time
func (t Timestamp) Time() time.Time {
	return time.Time(t)
}

// Request represents a proxied HTTP request
type Request struct {
	ID        string    `json:"id"`
	Method    string    `json:"method"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Path      string    `json:"path"`
	Query     string    `json:"query"`
	IsTLS     bool      `json:"isTls"`
	Raw       string    `json:"raw"`
	Response  *Response `json:"response"`
	CreatedAt Timestamp `json:"createdAt"`
}

// Response represents an HTTP response
type Response struct {
	StatusCode    int    `json:"statusCode"`
	Raw           string `json:"raw"`
	RoundtripTime int    `json:"roundtripTime"` // in milliseconds
}

// RequestSummary is a minimal representation of a request for list views
type RequestSummary struct {
	ID         string `json:"id"`
	Method     string `json:"method"`
	Host       string `json:"host"`
	Path       string `json:"path"`
	StatusCode int    `json:"statusCode,omitempty"`
}

// PageInfo contains pagination information
type PageInfo struct {
	HasNextPage     bool   `json:"hasNextPage"`
	HasPreviousPage bool   `json:"hasPreviousPage"`
	StartCursor     string `json:"startCursor"`
	EndCursor       string `json:"endCursor"`
}

// RequestsConnection is the paginated result of requests query
type RequestsConnection struct {
	Edges    []RequestEdge `json:"edges"`
	PageInfo PageInfo      `json:"pageInfo"`
}

// RequestEdge is a single edge in the requests connection
type RequestEdge struct {
	Cursor string  `json:"cursor"`
	Node   Request `json:"node"`
}

// AuthenticationRequest is the response from startAuthenticationFlow
type AuthenticationRequest struct {
	ID              string    `json:"id"`
	UserCode        string    `json:"userCode"`
	VerificationURL string    `json:"verificationUrl"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// AuthenticationToken contains the access and refresh tokens
type AuthenticationToken struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// AutomateSession represents an Automate fuzzing session
type AutomateSession struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Raw       string           `json:"raw"` // Base64 encoded request template
	CreatedAt Timestamp        `json:"createdAt"`
	Entries   []AutomateEntry  `json:"entries,omitempty"`
}

// AutomateEntry represents a single run/entry within an Automate session
type AutomateEntry struct {
	ID        string                        `json:"id"`
	Name      string                        `json:"name"`
	CreatedAt Timestamp                     `json:"createdAt"`
	Requests  *AutomateEntryRequestConnection `json:"requests,omitempty"`
}

// AutomateEntryRequestConnection is the paginated result of automate entry requests
type AutomateEntryRequestConnection struct {
	Edges    []AutomateEntryRequestEdge `json:"edges"`
	PageInfo PageInfo                   `json:"pageInfo"`
}

// AutomateEntryRequestEdge is a single edge in the automate entry requests connection
type AutomateEntryRequestEdge struct {
	Cursor string               `json:"cursor"`
	Node   AutomateEntryRequest `json:"node"`
}

// AutomateEntryRequest represents a single fuzzed request within an entry
type AutomateEntryRequest struct {
	SequenceID      string             `json:"sequenceId"`
	AutomateEntryID string             `json:"automateEntryId"`
	Payloads        []AutomatePayload  `json:"payloads"`
	Error           *string            `json:"error"`
	Request         *Request           `json:"request"`
}

// AutomatePayload represents a payload used in a fuzz request
type AutomatePayload struct {
	Raw      string `json:"raw"` // Base64 encoded payload
	Position *int   `json:"position"`
}

// AutomateSessionEdge is a single edge in the automate sessions connection
type AutomateSessionEdge struct {
	Cursor string          `json:"cursor"`
	Node   AutomateSession `json:"node"`
}

// AutomateSessionConnection is the paginated result of automate sessions query
type AutomateSessionConnection struct {
	Edges    []AutomateSessionEdge `json:"edges"`
	PageInfo PageInfo              `json:"pageInfo"`
}

// ReplaySession represents a Replay session
type ReplaySession struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	ActiveEntry *ReplayEntry  `json:"activeEntry,omitempty"`
	Entries     *ReplayEntryConnection `json:"entries,omitempty"`
}

// ReplayEntry represents a single entry in a Replay session
type ReplayEntry struct {
	ID         string      `json:"id"`
	Raw        string      `json:"raw"` // Base64 encoded request
	Connection *ConnectionInfo `json:"connection,omitempty"`
	Request    *Request    `json:"request,omitempty"`
}

// ConnectionInfo contains connection details
type ConnectionInfo struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	IsTLS bool   `json:"isTls"`
}

// ReplayEntryConnection is the paginated result of replay entries
type ReplayEntryConnection struct {
	Edges []ReplayEntryEdge `json:"edges"`
}

// ReplayEntryEdge is a single edge in the replay entries connection
type ReplayEntryEdge struct {
	Node ReplayEntry `json:"node"`
}

// ReplaySessionEdge is a single edge in the replay sessions connection
type ReplaySessionEdge struct {
	Cursor string        `json:"cursor"`
	Node   ReplaySession `json:"node"`
}

// ReplaySessionConnection is the paginated result of replay sessions
type ReplaySessionConnection struct {
	Edges []ReplaySessionEdge `json:"edges"`
}

// Finding represents a security finding
type Finding struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	Host        string    `json:"host"`
	Path        string    `json:"path"`
	Reporter    string    `json:"reporter"`
	Hidden      bool      `json:"hidden"`
	CreatedAt   Timestamp `json:"createdAt"`
	Request     *Request  `json:"request,omitempty"`
}

// FindingEdge is a single edge in the findings connection
type FindingEdge struct {
	Cursor string  `json:"cursor"`
	Node   Finding `json:"node"`
}

// FindingConnection is the paginated result of findings
type FindingConnection struct {
	Edges    []FindingEdge `json:"edges"`
	PageInfo PageInfo      `json:"pageInfo"`
}

// SitemapEntry represents an entry in the sitemap
type SitemapEntry struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	Kind           string   `json:"kind"` // DOMAIN, PATH, etc.
	ParentID       *string  `json:"parentId,omitempty"`
	HasDescendants bool     `json:"hasDescendants"`
	Request        *Request `json:"request,omitempty"`
}

// SitemapEntryEdge is a single edge in the sitemap entries connection
type SitemapEntryEdge struct {
	Node SitemapEntry `json:"node"`
}

// SitemapEntryConnection is the result of sitemap queries
type SitemapEntryConnection struct {
	Edges []SitemapEntryEdge `json:"edges"`
}

// Scope represents a target scope
type Scope struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Allowlist []string `json:"allowlist"`
	Denylist  []string `json:"denylist"`
	Indexed   bool     `json:"indexed"`
}
