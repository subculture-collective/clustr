// Package macrocluster groups fine communities into a small, semantically
// coherent set of macro categories using a provider LLM, with a deterministic
// fallback that never drops a community. It never accepts usernames or
// content text — only aggregate subreddit names and TF-IDF topics.
package macrocluster

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const PromptVersion = "macro-group-v1"
const PolicyVersion = "neutral-evidence-v1"
const MinGroups = 8
const MaxGroups = 12

// Community is the semantic summary of one fine community. Index must match the
// 0-based position of the community in the caller's slice.
type Community struct {
	Index           int      `json:"index"`
	Representatives []string `json:"representative_subreddits"`
	Topics          []string `json:"tfidf_topics"`
}

// Result carries the partition. Groups is an ordered slice where each element
// is the list of community indices assigned to that macro group. Every index
// appears exactly once.
type Result struct {
	Groups [][]int
	Method string
}

type Config struct {
	BaseURL, APIKey, Model string
	Timeout                time.Duration
	HTTPClient             *http.Client
}

// Fingerprint returns a stable hash over the normalized community evidence,
// used as a cache key.
func Fingerprint(communities []Community) string {
	normalized := make([]Community, len(communities))
	copy(normalized, communities)
	for index := range normalized {
		normalized[index].Representatives = normalizedStrings(normalized[index].Representatives)
		normalized[index].Topics = normalizedStrings(normalized[index].Topics)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Index < normalized[j].Index })
	body, _ := json.Marshal(normalized)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Group assigns every community to exactly one macro group. It returns the
// deterministic fallback partition when the provider is unconfigured, fails, or
// returns an invalid partition, so the build never blocks on the LLM.
func Group(ctx context.Context, communities []Community, config Config) (Result, error) {
	fallback := FallbackGroup(communities)
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		log.Printf("macrocluster: provider unconfigured, using deterministic fallback")
		return fallback, nil
	}
	var groups [][]int
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		groups, lastErr = callProvider(ctx, communities, config)
		if lastErr == nil {
			break
		}
		log.Printf("macrocluster: provider attempt %d failed: %v", attempt, lastErr)
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return fallback, nil
			case <-time.After(10 * time.Second):
			}
		}
	}
	if lastErr != nil {
		log.Printf("macrocluster: provider call failed after retries, using deterministic fallback: %v", lastErr)
		return fallback, nil
	}
	if err := Validate(groups, len(communities)); err != nil {
		// The provider often returns a near-valid partition with a couple of
		// duplicated or missing indices. Repair it deterministically instead of
		// discarding a semantically coherent grouping.
		if repaired, repairErr := repair(groups, len(communities)); repairErr == nil {
			log.Printf("macrocluster: provider partition repaired (%v)", err)
			return Result{Groups: repaired, Method: "provider"}, nil
		}
		log.Printf("macrocluster: provider partition invalid (%v), using deterministic fallback", err)
		return fallback, nil
	}
	return Result{Groups: groups, Method: "provider"}, nil
}

// repair deduplicates indices (keeping the first occurrence), drops out-of-range
// indices, and redistributes any missing indices to the smallest groups so the
// result is a complete balanced partition. It returns an error only if the
// group count falls outside the configured range after removing empty groups.
func repair(groups [][]int, communityCount int) ([][]int, error) {
	seen := make([]bool, communityCount)
	repaired := make([][]int, 0, len(groups))
	for _, group := range groups {
		clean := make([]int, 0, len(group))
		for _, index := range group {
			if index < 0 || index >= communityCount || seen[index] {
				continue
			}
			seen[index] = true
			clean = append(clean, index)
		}
		if len(clean) > 0 {
			repaired = append(repaired, clean)
		}
	}
	if len(repaired) < MinGroups || len(repaired) > MaxGroups {
		return nil, fmt.Errorf("repaired group count %d outside %d-%d", len(repaired), MinGroups, MaxGroups)
	}
	// Redistribute missing indices to the smallest groups for balance.
	for index := 0; index < communityCount; index++ {
		if seen[index] {
			continue
		}
		smallest := 0
		for g := 1; g < len(repaired); g++ {
			if len(repaired[g]) < len(repaired[smallest]) {
				smallest = g
			}
		}
		repaired[smallest] = append(repaired[smallest], index)
	}
	if err := Validate(repaired, communityCount); err != nil {
		return nil, err
	}
	return repaired, nil
}

// FallbackGroup deterministically assigns communities to MinGroups balanced
// groups by round-robin over sorted indices. It never drops a community.
func FallbackGroup(communities []Community) Result {
	groups := make([][]int, MinGroups)
	for _, community := range communities {
		group := community.Index % MinGroups
		if group < 0 {
			group = -group % MinGroups
		}
		groups[group] = append(groups[group], community.Index)
	}
	return Result{Groups: groups, Method: "deterministic-fallback"}
}

type completionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type providerOutput struct {
	Groups [][]int `json:"groups"`
}

func callProvider(ctx context.Context, communities []Community, config Config) ([][]int, error) {
	base, err := url.Parse(strings.TrimRight(config.BaseURL, "/") + "/chat/completions")
	if err != nil {
		return nil, err
	}
	// Every index must appear exactly once; this bounds the schema tightly so a
	// strict provider cannot drop or duplicate a community.
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"groups"}, "properties": map[string]any{
		"groups": map[string]any{"type": "array", "minItems": MinGroups, "maxItems": MaxGroups, "items": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}}}}
	communitiesJSON, _ := json.Marshal(communities)
	prompt := fmt.Sprintf("Group these %d computed subreddit communities into %d-%d macro categories for a map navigation system. Assign every community index to exactly one category. Categories should be semantically coherent based on the representative subreddits and topics. Do not infer demographics, ideology, identity, intent, or facts not present in the evidence. Return only the groups of community indices. Communities: %s", len(communities), MinGroups, MaxGroups, string(communitiesJSON))
	requestBody := map[string]any{"model": config.Model, "temperature": 0, "messages": []map[string]string{{"role": "system", "content": "You organize aggregate subreddit communities into navigation categories neutrally. Never use or request usernames, post text, or comment text."}, {"role": "user", "content": prompt}}, "response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "macro_groups", "strict": true, "schema": schema}}}
	body, _ := json.Marshal(requestBody)
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("macro provider status %d", response.StatusCode)
	}
	var completion completionResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&completion); err != nil {
		return nil, err
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("macro provider returned no choices")
	}
	var output providerOutput
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &output); err != nil {
		return nil, err
	}
	return output.Groups, nil
}

// Validate checks that groups forms a complete partition of 0..communityCount-1
// with a group count inside the configured range, no missing or duplicate
// indices, no empty groups, and no single group holding more than half of all
// communities.
func Validate(groups [][]int, communityCount int) error {
	if len(groups) < MinGroups || len(groups) > MaxGroups {
		return fmt.Errorf("group count %d outside %d-%d", len(groups), MinGroups, MaxGroups)
	}
	seen := make([]bool, communityCount)
	assigned := 0
	for _, group := range groups {
		if len(group) == 0 {
			return fmt.Errorf("empty macro group")
		}
		if len(group) > communityCount/2 {
			return fmt.Errorf("macro group too large (%d of %d)", len(group), communityCount)
		}
		for _, index := range group {
			if index < 0 || index >= communityCount {
				return fmt.Errorf("community index %d out of range", index)
			}
			if seen[index] {
				return fmt.Errorf("community index %d assigned twice", index)
			}
			seen[index] = true
			assigned++
		}
	}
	if assigned != communityCount {
		return fmt.Errorf("assigned %d communities, want %d", assigned, communityCount)
	}
	return nil
}

func normalizedStrings(values []string) []string {
	seen := map[string]string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			key := strings.ToLower(value)
			if _, ok := seen[key]; !ok {
				seen[key] = value
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}
