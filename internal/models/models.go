// Package models provides a generic, provider-agnostic helper to list the
// model IDs available from an OpenAI-style endpoint (e.g. a LiteLLM proxy).
//
// The dedup rule implemented here MUST stay in sync with the equivalent logic
// embedded in the generated pi extension (internal/target/picli), since both
// consume the same endpoint and need to present the same canonical model IDs.
package models

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// fetchTimeout bounds the HTTP requests so a hung endpoint doesn't block the CLI.
const fetchTimeout = 10 * time.Second

// versionDash matches a dash that separates version-number components, e.g. the
// dashes in "claude-opus-4-8" (between digits). Used to treat dashed version
// forms as equivalent to dotted forms ("claude-opus-4.8").
var versionDash = regexp.MustCompile(`(\d)-(\d)`)

// normalizeID returns a canonical key used to group equivalent model IDs.
// It strips a leading "<vendor>:" prefix and converts version-separating dashes
// to dots so that "aws:claude-opus-4-8", "claude-opus-4-8" and
// "claude-opus-4.8" all map to the same key. Date-pinned IDs (e.g.
// "claude-haiku-4-5-20251001") normalize distinctly and are preserved.
func normalizeID(id string) string {
	// Strip a leading "<vendor>:" prefix (everything up to the first colon).
	if idx := strings.Index(id, ":"); idx >= 0 {
		id = id[idx+1:]
	}
	// Convert version-separating dashes ("4-8" -> "4.8"). Apply repeatedly to
	// handle chains like "4-5-1" -> "4.5.1".
	for {
		next := versionDash.ReplaceAllString(id, "$1.$2")
		if next == id {
			break
		}
		id = next
	}
	return id
}

// Dedup groups the input IDs by their normalized key and keeps one
// representative per group, preferring the dotted, unprefixed candidate.
// The output is sorted alphabetically for stable display.
func Dedup(ids []string) []string {
	groups := map[string][]string{}
	order := []string{}
	for _, id := range ids {
		key := normalizeID(id)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], id)
	}

	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, pickRepresentative(groups[key]))
	}
	sort.Strings(out)
	return out
}

// pickRepresentative chooses the preferred ID from a group of equivalent IDs:
// prefer unprefixed (no "vendor:") over prefixed, and dotted over dashed. Ties
// break alphabetically for determinism.
func pickRepresentative(candidates []string) string {
	best := candidates[0]
	for _, c := range candidates[1:] {
		if betterCandidate(c, best) {
			best = c
		}
	}
	return best
}

func betterCandidate(a, b string) bool {
	sa, sb := candidateScore(a), candidateScore(b)
	if sa != sb {
		return sa > sb
	}
	return a < b
}

// candidateScore ranks a candidate: higher is more preferred.
//   +2 if unprefixed (no "vendor:")
//   +1 if dotted (contains a dot, e.g. "4.8" over "4-8")
func candidateScore(id string) int {
	score := 0
	if !strings.Contains(id, ":") {
		score += 2
	}
	if strings.Contains(id, ".") {
		score++
	}
	return score
}

// openAIModelsResponse mirrors the OpenAI-style /v1/models response.
type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// FetchModelIDs fetches the available model IDs from an OpenAI-style endpoint.
// It tries "{endpoint}/v1/models" first, then falls back to "{endpoint}/models"
// on any non-200 response. The apiKey (when non-empty) is sent as a bearer
// token, along with any additional headers. The returned IDs are deduped and
// sorted.
func FetchModelIDs(endpoint, apiKey string, headers map[string]string) ([]string, error) {
	base := strings.TrimSuffix(endpoint, "/")
	client := &http.Client{Timeout: fetchTimeout}

	ids, err := fetchFrom(client, base+"/v1/models", apiKey, headers)
	if err == nil {
		return Dedup(ids), nil
	}
	firstErr := err

	ids, err = fetchFrom(client, base+"/models", apiKey, headers)
	if err == nil {
		return Dedup(ids), nil
	}

	return nil, fmt.Errorf("fetching models failed: %v (fallback: %v)", firstErr, err)
}

func fetchFrom(client *http.Client, url, apiKey string, headers map[string]string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned status %d", url, resp.StatusCode)
	}

	var mr openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", url, err)
	}

	ids := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s returned no models", url)
	}
	return ids, nil
}
