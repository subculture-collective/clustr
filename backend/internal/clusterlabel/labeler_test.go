package clusterlabel_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/clusterlabel"
)

func TestNoProviderPublishesDeterministicEvidenceLabel(t *testing.T) {
	evidence := clusterlabel.Evidence{Representatives: []string{"DIY", "woodworking", "3Dprinting"}, Topics: []string{"maker", "woodworking"}}
	first, err := clusterlabel.Generate(context.Background(), evidence, clusterlabel.Config{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := clusterlabel.Generate(context.Background(), evidence, clusterlabel.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("fallback changed: %+v != %+v", first, second)
	}
	if first.Method != "deterministic-fallback" || first.DisplayName == "" || first.EvidenceLabel != "DIY · woodworking · 3Dprinting" {
		t.Fatalf("unexpected fallback: %+v", first)
	}
}

func TestProviderSchemaConstrainsGrounding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ResponseFormat struct {
				JSONSchema struct {
					Schema struct {
						Properties map[string]json.RawMessage `json:"properties"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		var grounding struct {
			Items struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		}
		if err := json.Unmarshal(request.ResponseFormat.JSONSchema.Schema.Properties["evidence_grounding"], &grounding); err != nil {
			t.Fatal(err)
		}
		if len(grounding.Items.Enum) != 4 {
			t.Fatalf("grounding enum = %v", grounding.Items.Enum)
		}
		if _, present := request.ResponseFormat.JSONSchema.Schema.Properties["method_version"]; present {
			t.Fatal("method_version must be stamped locally, not requested from the provider")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"display_name\":\"Maker Commons\",\"evidence_grounding\":[\"woodworking\",\"maker\"],\"confidence\":0.9}"}}]}`))
	}))
	defer server.Close()

	evidence := clusterlabel.Evidence{Representatives: []string{"woodworking", "DIY"}, Topics: []string{"maker", "tools"}}
	result, err := clusterlabel.Generate(context.Background(), evidence, clusterlabel.Config{BaseURL: server.URL, APIKey: "test-key", Model: "test-model", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "provider" || result.DisplayName != "Maker Commons" || result.Grounding != "woodworking · maker" {
		t.Fatalf("unexpected provider result: %+v", result)
	}
	if result.MethodVersion != clusterlabel.PromptVersion {
		t.Fatalf("method version = %q, want %q", result.MethodVersion, clusterlabel.PromptVersion)
	}
}

func TestProviderFailureChangesNoDeterministicEvidence(t *testing.T) {
	evidence := clusterlabel.Evidence{Representatives: []string{"DIY", "woodworking"}, Topics: []string{"maker", "tools"}}
	want, err := clusterlabel.Generate(context.Background(), evidence, clusterlabel.Config{})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := clusterlabel.Generate(context.Background(), evidence, clusterlabel.Config{
		BaseURL: "http://127.0.0.1:1", APIKey: "unreachable", Model: "test", Timeout: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed != want {
		t.Fatalf("provider failure changed deterministic label evidence: got %+v want %+v", failed, want)
	}
}

func TestEvidenceFingerprintIgnoresInputOrdering(t *testing.T) {
	a := clusterlabel.Evidence{Representatives: []string{"woodworking", "DIY"}, Topics: []string{"tools", "maker"}}
	b := clusterlabel.Evidence{Representatives: []string{"DIY", "woodworking"}, Topics: []string{"maker", "tools"}}
	if clusterlabel.Fingerprint(a) != clusterlabel.Fingerprint(b) {
		t.Fatal("equivalent evidence must share a cache fingerprint")
	}
}
