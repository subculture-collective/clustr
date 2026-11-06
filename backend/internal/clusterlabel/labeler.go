// Package clusterlabel produces constrained automatic community names from
// aggregate subreddit evidence. It never accepts usernames or content text.
package clusterlabel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const PromptVersion = "cluster-label-v1"
const PolicyVersion = "neutral-evidence-v1"

type Evidence struct {
	Representatives     []string           `json:"representative_subreddits"`
	Topics              []string           `json:"tfidf_topics"`
	NeighborMacroGroups []string           `json:"neighbor_macro_groups,omitempty"`
	Metrics             map[string]float64 `json:"aggregate_metrics,omitempty"`
}

type Config struct {
	BaseURL, APIKey, Model string
	Timeout                time.Duration
	HTTPClient             *http.Client
}
type Result struct {
	DisplayName, EvidenceLabel, Method, MethodVersion string
	Confidence                                        float64
	Grounding                                         string
}

func Fingerprint(e Evidence) string {
	normalized := e
	normalized.Representatives = normalizedStrings(e.Representatives)
	normalized.Topics = normalizedStrings(e.Topics)
	normalized.NeighborMacroGroups = normalizedStrings(e.NeighborMacroGroups)
	body, _ := json.Marshal(normalized)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func Generate(ctx context.Context, e Evidence, config Config) (Result, error) {
	fallback := fallbackResult(e)
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		return fallback, nil
	}
	provider, err := callProvider(ctx, e, config)
	if err != nil {
		return fallback, nil
	}
	if err := validate(provider, e); err != nil {
		return fallback, nil
	}
	return provider, nil
}

func fallbackResult(e Evidence) Result {
	representatives := uniqueStrings(e.Representatives)
	topics := uniqueStrings(e.Topics)
	evidenceParts := representatives
	if len(evidenceParts) > 3 {
		evidenceParts = evidenceParts[:3]
	}
	if len(evidenceParts) == 0 {
		evidenceParts = topics
		if len(evidenceParts) > 3 {
			evidenceParts = evidenceParts[:3]
		}
	}
	evidenceLabel := strings.Join(evidenceParts, " · ")
	if evidenceLabel == "" {
		evidenceLabel = "Unplaced evidence"
	}
	nameParts := topics
	if len(nameParts) == 0 {
		nameParts = representatives
	}
	if len(nameParts) > 3 {
		nameParts = nameParts[:3]
	}
	if len(nameParts) == 0 {
		nameParts = []string{"Unplaced"}
	}
	for index := range nameParts {
		nameParts[index] = titleWord(nameParts[index])
	}
	if len(nameParts) == 1 {
		nameParts = append(nameParts, "Community")
	}
	return Result{DisplayName: strings.Join(nameParts, " "), EvidenceLabel: evidenceLabel, Method: "deterministic-fallback", MethodVersion: PromptVersion, Confidence: 0, Grounding: evidenceLabel}
}

type providerOutput struct {
	DisplayName       string   `json:"display_name"`
	EvidenceGrounding []string `json:"evidence_grounding"`
	Confidence        float64  `json:"confidence"`
}
type completionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func callProvider(ctx context.Context, e Evidence, config Config) (Result, error) {
	base, err := url.Parse(strings.TrimRight(config.BaseURL, "/") + "/chat/completions")
	if err != nil {
		return Result{}, err
	}
	allowedGrounding := uniqueStrings(append(append(append([]string{}, e.Representatives...), e.Topics...), e.NeighborMacroGroups...))
	if len(allowedGrounding) == 0 {
		return Result{}, errors.New("label evidence is empty")
	}
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"display_name", "evidence_grounding", "confidence"}, "properties": map[string]any{
		"display_name": map[string]any{"type": "string"}, "evidence_grounding": map[string]any{"type": "array", "minItems": 1, "maxItems": 6, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": allowedGrounding}}, "confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1}}}
	evidenceJSON, _ := json.Marshal(e)
	prompt := "Name this computed subreddit cluster. Return a neutral 2-5 word noun phrase. For evidence_grounding, select only exact, unmodified strings from the supplied representative_subreddits, tfidf_topics, or neighbor_macro_groups arrays. Ground the name only in that evidence and the aggregate metrics. Do not infer demographics, ideology, identity, intent, or facts not present. Do not merely repeat one giant subreddit. Evidence: " + string(evidenceJSON)
	requestBody := map[string]any{"model": config.Model, "temperature": 0, "messages": []map[string]string{{"role": "system", "content": "You label aggregate communities neutrally. Never use or request usernames, post text, or comment text."}, {"role": "user", "content": prompt}}, "response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "cluster_label", "strict": true, "schema": schema}}}
	body, _ := json.Marshal(requestBody)
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Result{}, fmt.Errorf("label provider status %d", response.StatusCode)
	}
	var completion completionResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&completion); err != nil {
		return Result{}, err
	}
	if len(completion.Choices) == 0 {
		return Result{}, errors.New("label provider returned no choices")
	}
	var output providerOutput
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &output); err != nil {
		return Result{}, err
	}
	return Result{DisplayName: strings.TrimSpace(output.DisplayName), EvidenceLabel: fallbackResult(e).EvidenceLabel, Method: "provider", MethodVersion: PromptVersion, Confidence: output.Confidence, Grounding: strings.Join(output.EvidenceGrounding, " · ")}, nil
}

func validate(result Result, e Evidence) error {
	words := strings.Fields(result.DisplayName)
	if len(words) < 2 || len(words) > 5 {
		return errors.New("display name must contain 2-5 words")
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return errors.New("invalid confidence")
	}
	if result.MethodVersion != PromptVersion || result.Grounding == "" {
		return errors.New("missing method or grounding")
	}
	evidence := strings.ToLower(strings.Join(append(append(append([]string{}, e.Representatives...), e.Topics...), e.NeighborMacroGroups...), " "))
	for _, term := range strings.Split(result.Grounding, " · ") {
		if term != "" && !strings.Contains(evidence, strings.ToLower(term)) {
			return errors.New("unsupported grounding")
		}
	}
	for _, term := range []string{"retard", "nigger", "faggot"} {
		if strings.Contains(strings.ToLower(result.DisplayName), term) && !strings.Contains(evidence, term) {
			return errors.New("unsupported slur")
		}
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
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	return out
}
func titleWord(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	if runes[0] >= 'a' && runes[0] <= 'z' {
		runes[0] -= 'a' - 'A'
	}
	return string(runes)
}
