package clusterlabel_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestProviderSchemaConstrainsGroundingAndMethodVersion(t *testing.T) {
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
		var method struct {
			Const string `json:"const"`
		}
		if err := json.Unmarshal(request.ResponseFormat.JSONSchema.Schema.Properties["method_version"], &method); err != nil {
			t.Fatal(err)
		}
		if method.Const != clusterlabel.PromptVersion {
			t.Fatalf("method const = %q", method.Const)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"display_name\":\"Maker Commons\",\"evidence_grounding\":[\"woodworking\",\"maker\"],\"confidence\":0.9,\"method_version\":\"cluster-label-v1\"}"}}]}`))
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
}

func TestEvidenceFingerprintIgnoresInputOrdering(t *testing.T) {
	a := clusterlabel.Evidence{Representatives: []string{"woodworking", "DIY"}, Topics: []string{"tools", "maker"}}
	b := clusterlabel.Evidence{Representatives: []string{"DIY", "woodworking"}, Topics: []string{"maker", "tools"}}
	if clusterlabel.Fingerprint(a) != clusterlabel.Fingerprint(b) {
		t.Fatal("equivalent evidence must share a cache fingerprint")
	}
}
