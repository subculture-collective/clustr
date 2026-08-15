package clusterlabel_test

import (
	"context"
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

func TestEvidenceFingerprintIgnoresInputOrdering(t *testing.T) {
	a := clusterlabel.Evidence{Representatives: []string{"woodworking", "DIY"}, Topics: []string{"tools", "maker"}}
	b := clusterlabel.Evidence{Representatives: []string{"DIY", "woodworking"}, Topics: []string{"maker", "tools"}}
	if clusterlabel.Fingerprint(a) != clusterlabel.Fingerprint(b) {
		t.Fatal("equivalent evidence must share a cache fingerprint")
	}
}
