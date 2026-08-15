package affinity_test

import (
	"testing"

	"github.com/onnwee/reddit-cluster-map/backend/internal/affinity"
)

func TestHistoricalDefaultPriorOnlyChangesRepresentativeScore(t *testing.T){
	ordinary:=affinity.RepresentativeScore("DistinctiveNiche",.9,.8,.9,.1)
	defaultScore:=affinity.RepresentativeScore("AskReddit",.9,.8,.9,.1)
	if defaultScore!=ordinary*.5{t.Fatalf("default prior=%v, want %v",defaultScore,ordinary*.5)}
	defaults,checksum:=affinity.HistoricalDefaults();if !defaults["askreddit"]||len(checksum)!=64{t.Fatalf("dataset provenance missing: checksum=%q",checksum)}
}
