package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
// the THREAD's GraphQL node id — a PullRequestReviewThread node id, the `id`
// the reviewThreads listing reports, NOT a review comment's own node id.
const gqlResolveThread = `mutation($threadId: ID!) {
  resolveReviewThread(input: {threadId: $threadId}) {
    thread {
      id
      isResolved
    }
  }
}`

// gqlReviewThreads reads the PR's review threads with their resolution states
// (dossier C1/C2: resolution is GraphQL-only, and the REST listing cannot see
// it). The port's Thread.Resolved is data the engine consumes — a reviewer's
// resolution must read back resolved — so every bot-thread listing consults
// this query and maps each review comment's numeric id to its thread's
// isResolved state.
const gqlReviewThreads = `query($owner: String!, $name: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $cursor) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          isResolved
          comments(first: 10) {
            nodes {
              databaseId
            }
          }
        }
      }
    }
  }
}`

// reviewThreadIndex is what the reviewThreads listing yields: every review
// comment's numeric REST id mapped to (a) its thread's isResolved state and
// (b) its THREAD's GraphQL node id — the handle resolveReviewThread addresses
// threads by (a thread id is a PullRequestReviewThread node id, a DIFFERENT
// object from the comment's own node id).
type reviewThreadIndex struct {
	states    map[int64]bool
	threadIDs map[int64]string
}

// threadResolutionStates reads the PR's review threads via GraphQL and returns
// each review comment's numeric REST id mapped to its thread's isResolved
// state. Comments this query does not mention are open (the conservative
// reading); a transport failure propagates — an unreadable resolution state is
// not silently an open one. The cursor loop is capped like every listing read
// (maxListPages, fail-closed exhaustion).
func (c *Client) threadResolutionStates(project, mr string) (map[int64]bool, error) {
	idx, err := c.reviewThreadIndex(project, mr)
	if err != nil {
		return nil, err
	}
	return idx.states, nil
}

// threadNodeIDFor returns the THREAD node id (a PullRequestReviewThread node
// id) carrying the review comment `commentID`, via the reviewThreads GraphQL
// read the resolution states come from. A comment no thread carries is an
// error, never a guess: resolveReviewThread addresses the THREAD object, and
// resolving a comment's own node id would resolve nothing (or someone else's
// thread).
func (c *Client) threadNodeIDFor(project, mr string, commentID int64) (string, error) {
	idx, err := c.reviewThreadIndex(project, mr)
	if err != nil {
		return "", err
	}
	threadID := idx.threadIDs[commentID]
	if threadID == "" {
		return "", fmt.Errorf(
			"github: no review thread on %s#%s carries comment %d — the resolveReviewThread mutation addresses the THREAD node id (a PullRequestReviewThread), not the comment's own node id, and no listing reports this thread",
			project, mr, commentID)
	}
	return threadID, nil
}

// reviewThreadIndex reads the PR's review threads via GraphQL and indexes every
// listed comment's numeric id to its thread's node id and isResolved state.
// Comments this query does not mention are absent from the index (the caller
// decides the conservative reading); a transport failure propagates — an
// unreadable resolution state is not silently an open one. The cursor loop is
// capped like every listing read (maxListPages, fail-closed exhaustion).
func (c *Client) reviewThreadIndex(project, mr string) (reviewThreadIndex, error) {
	owner, name, err := ownerRepo(project)
	if err != nil {
		return reviewThreadIndex{}, err
	}
	number, err := strconv.Atoi(mr)
	if err != nil {
		return reviewThreadIndex{}, fmt.Errorf("github: PR number %q is not numeric — review threads cannot be addressed", mr)
	}
	idx := reviewThreadIndex{states: make(map[int64]bool), threadIDs: make(map[int64]string)}
	cursor := ""
	for page := 1; ; page++ {
		if page > maxListPages {
			return reviewThreadIndex{}, fmt.Errorf(
				"github: list review threads %s#%s: pagination cap of %d pages reached without a short page — refusing to reconcile against an unreadable resolution state",
				project, mr, maxListPages)
		}
		vars := map[string]any{"owner": owner, "name": name, "number": number}
		if cursor != "" {
			vars["cursor"] = cursor
		}
		data, err := c.gqlDo(c.ctx, gqlReviewThreads, vars)
		if err != nil {
			return reviewThreadIndex{}, fmt.Errorf("github: review threads %s#%s: %w", project, mr, err)
		}
		var payload struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID         string `json:"id"`
							IsResolved bool   `json:"isResolved"`
							Comments   struct {
								Nodes []struct {
									DatabaseID int64 `json:"databaseId"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return reviewThreadIndex{}, fmt.Errorf("github: decode reviewThreads %s#%s: %w", project, mr, err)
		}
		threads := payload.Repository.PullRequest.ReviewThreads
		for _, node := range threads.Nodes {
			for _, comment := range node.Comments.Nodes {
				if comment.DatabaseID == 0 {
					continue
				}
				idx.states[comment.DatabaseID] = node.IsResolved
				idx.threadIDs[comment.DatabaseID] = node.ID
			}
		}
		if !threads.PageInfo.HasNextPage {
			return idx, nil
		}
		cursor = threads.PageInfo.EndCursor
	}
}

// ownerRepo splits an "owner/repo" project address into its GraphQL halves.
func ownerRepo(project string) (string, string, error) {
	parts, err := repoParts(project)
	if err != nil {
		return "", "", err
	}
	i := strings.Index(parts, "/")
	return parts[:i], parts[i+1:], nil
}
