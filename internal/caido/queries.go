package caido

const (
	// RequestsQuery is the GraphQL query for listing requests
	RequestsQuery = `
query Requests($first: Int, $after: String, $filter: HTTPQL) {
  requests(first: $first, after: $after, filter: $filter) {
    edges {
      cursor
      node {
        id
        method
        host
        port
        path
        query
        isTls
        createdAt
        response {
          statusCode
          roundtripTime
        }
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
`

	// RequestQuery is the GraphQL query for getting a single request by ID
	RequestQuery = `
query Request($id: ID!) {
  request(id: $id) {
    id
    method
    host
    port
    path
    query
    isTls
    raw
    createdAt
    response {
      statusCode
      raw
      roundtripTime
    }
  }
}
`

	// StartAuthenticationFlowMutation initiates the OAuth flow
	StartAuthenticationFlowMutation = `
mutation StartAuthenticationFlow {
  startAuthenticationFlow {
    request {
      id
      userCode
      verificationUrl
      expiresAt
    }
    error {
      __typename
    }
  }
}
`

	// RefreshAuthenticationTokenMutation refreshes the access token
	RefreshAuthenticationTokenMutation = `
mutation RefreshAuthenticationToken($refreshToken: Token!) {
  refreshAuthenticationToken(refreshToken: $refreshToken) {
    token {
      accessToken
      refreshToken
      expiresAt
    }
    error {
      __typename
    }
  }
}
`

	// AutomateSessionsQuery lists all Automate sessions
	AutomateSessionsQuery = `
query AutomateSessions {
  automateSessions {
    edges {
      cursor
      node {
        id
        name
        createdAt
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
`

	// AutomateSessionQuery gets a single Automate session with entries
	AutomateSessionQuery = `
query AutomateSession($id: ID!) {
  automateSession(id: $id) {
    id
    name
    raw
    createdAt
    entries {
      id
      name
      createdAt
    }
  }
}
`

	// AutomateEntryQuery gets a single Automate entry with fuzz results
	AutomateEntryQuery = `
query AutomateEntry($id: ID!, $first: Int, $after: String) {
  automateEntry(id: $id) {
    id
    name
    createdAt
    requests(first: $first, after: $after) {
      edges {
        cursor
        node {
          sequenceId
          automateEntryId
          payloads {
            raw
            position
          }
          error
          request {
            id
            method
            host
            port
            path
            query
            isTls
            raw
            response {
              statusCode
              raw
              roundtripTime
            }
          }
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
`

	// ReplaySessionsQuery lists all Replay sessions
	ReplaySessionsQuery = `
query ReplaySessions {
  replaySessions {
    edges {
      cursor
      node {
        id
        name
        activeEntry {
          id
        }
      }
    }
  }
}
`

	// ReplaySessionQuery gets a single Replay session with entries
	ReplaySessionQuery = `
query ReplaySession($id: ID!) {
  replaySession(id: $id) {
    id
    name
    activeEntry {
      id
    }
    entries {
      edges {
        node {
          id
          raw
        }
      }
    }
  }
}
`

	// ReplayEntryQuery gets a single Replay entry with request/response
	ReplayEntryQuery = `
query ReplayEntry($id: ID!) {
  replayEntry(id: $id) {
    id
    raw
    connection {
      host
      port
      isTls
    }
    request {
      id
      method
      host
      port
      path
      query
      isTls
      raw
      response {
        statusCode
        raw
        roundtripTime
      }
    }
  }
}
`

	// StartReplayTaskMutation sends a request via Replay
	StartReplayTaskMutation = `
mutation StartReplayTask($sessionId: ID!, $input: StartReplayTaskInput!) {
  startReplayTask(sessionId: $sessionId, input: $input) {
    task {
      id
    }
    error {
      __typename
      ... on OtherUserError {
        code
      }
    }
  }
}
`

	// CreateReplaySessionMutation creates a new replay session
	CreateReplaySessionMutation = `
mutation CreateReplaySession($input: CreateReplaySessionInput!) {
  createReplaySession(input: $input) {
    session {
      id
      name
    }
    error {
      __typename
    }
  }
}
`

	// FindingsQuery lists all findings
	FindingsQuery = `
query Findings($first: Int, $after: String, $filter: HTTPQL) {
  findings(first: $first, after: $after, filter: $filter) {
    edges {
      cursor
      node {
        id
        title
        description
        host
        path
        reporter
        hidden
        createdAt
        request {
          id
        }
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
`

	// CreateFindingMutation creates a new finding
	CreateFindingMutation = `
mutation CreateFinding($requestId: ID!, $input: CreateFindingInput!) {
  createFinding(requestId: $requestId, input: $input) {
    finding {
      id
      title
      description
      host
      path
      reporter
    }
    error {
      __typename
    }
  }
}
`

	// SitemapRootEntriesQuery gets root sitemap entries
	SitemapRootEntriesQuery = `
query SitemapRootEntries {
  sitemapRootEntries {
    edges {
      node {
        id
        label
        kind
        hasDescendants
      }
    }
  }
}
`

	// SitemapDescendantEntriesQuery gets children of a sitemap entry
	SitemapDescendantEntriesQuery = `
query SitemapDescendantEntries($parentId: ID!, $depth: SitemapDescendantsDepth!) {
  sitemapDescendantEntries(parentId: $parentId, depth: $depth) {
    edges {
      node {
        id
        label
        kind
        parentId
        hasDescendants
        request {
          id
          method
          path
          response {
            statusCode
          }
        }
      }
    }
  }
}
`

	// ScopesQuery lists all scopes
	ScopesQuery = `
query Scopes {
  scopes {
    id
    name
    allowlist
    denylist
    indexed
  }
}
`

	// CreateScopeMutation creates a new scope
	CreateScopeMutation = `
mutation CreateScope($input: CreateScopeInput!) {
  createScope(input: $input) {
    scope {
      id
      name
    }
    error {
      __typename
    }
  }
}
`
)
