package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// graphql.go is the GraphQL transport, reserved for thread resolution only
// (dossier §4: thread resolution is GraphQL-only; everything else is REST —
// S00 judgment call (c)). The port names no transport; this file is
// adapter-internal freedom. The same secret-hygiene rule applies: no credential
// material in any error, and the response read is byte-bounded.

// gqlDo performs one GraphQL call and returns the `data` envelope. Errors are
// surfaced as a hard error naming the operation, never a sentinel and never a
// secret.
func (c *Client) gqlDo(ctx context.Context, query string, variables map[string]any) (json.RawMessage, error) {
	// The GraphQL URL is the configured REST endpoint + /graphql: the public
	// endpoint (https://api.github.com → https://api.github.com/graphql) and an
	// httptest-backed harness's /graphql route are both served there — the
	// constructor seam that keeps the GraphQL path hermetic under test.
	gqlEndpoint := strings.TrimRight(c.endpoint, "/") + "/graphql"
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, errors.New("github: graphql: request encoding failed")
	}
	bearer, err := c.bearer(ctx)
	if err != nil {
		return nil, err
	}
	ctx2, cancel := context.WithTimeout(ctx, c.retry.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx2, http.MethodPost, gqlEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("github: graphql: build request failed")
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: graphql: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := readBounded(resp.Body, maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("github: graphql: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: graphql: unexpected status %d", resp.StatusCode)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlErrorShape `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("github: graphql: decode: %w", err)
	}
	// A GraphQL error envelope carries data:null alongside the errors — treat
	// that (JSON null, not just a missing key) as no data.
	if len(envelope.Errors) > 0 && (envelope.Data == nil || string(envelope.Data) == "null") {
		return nil, fmt.Errorf("github: graphql: %s", strings.Join(errorMessages(envelope.Errors), "; "))
	}
	return envelope.Data, nil
}

// errorMessages joins GraphQL error messages.
func errorMessages(gqlErrs []gqlErrorShape) []string {
	out := make([]string, 0, len(gqlErrs))
	for _, e := range gqlErrs {
		out = append(out, e.Message)
	}
	return out
}

type gqlErrorShape struct{ Message string }

// gqlResolveThread is the GraphQL mutation resolving ONE review thread
// (dossier §4: thread resolution is GraphQL-only). The thread is addressed by
// its GraphQL node id — the REST listing's `node_id`, which the adapter
// remembers in nodeIDs at every listing and creation.
const gqlResolveThread = `mutation($threadId: ID!) {
  resolveReviewThread(input: {threadId: $threadId}) {
    thread {
      id
      isResolved
    }
  }
}`
