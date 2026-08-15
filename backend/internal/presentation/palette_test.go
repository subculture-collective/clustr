package presentation_test

import (
	"testing"

	"github.com/onnwee/reddit-cluster-map/backend/internal/presentation"
)

func TestBridgeRequiresBothAbsoluteAndRelativeAffinity(t *testing.T) {
	if !presentation.IsBridge(.8, .45) {
		t.Fatal(".45 is at least .30 and .55 of .8")
	}
	if presentation.IsBridge(.8, .29) {
		t.Fatal("secondary affinity below .30 must not bridge")
	}
	if presentation.IsBridge(.8, .40) {
		t.Fatal("secondary affinity below .55 of primary must not bridge")
	}
}

func TestPaletteHasTwelveDistinctStableRoles(t *testing.T) {
	palette := presentation.MacroPalette()
	if len(palette) != 12 {
		t.Fatalf("palette has %d roles", len(palette))
	}
	seen := map[string]bool{}
	for _, color := range palette {
		if seen[color] {
			t.Fatalf("duplicate palette role %s", color)
		}
		seen[color] = true
	}
}

func TestClusterShadeIsBoundedStableAndPreservesRole(t *testing.T) {
	dark := presentation.ClusterShade("#68DCFF", -1, .5)
	light := presentation.ClusterShade("#68DCFF", 1, .5)
	if dark == light || len(dark) != 7 || len(light) != 7 || dark[0] != '#' || light[0] != '#' {
		t.Fatalf("unexpected shades dark=%s light=%s", dark, light)
	}
	if got := presentation.ClusterShade("invalid", 0, 1); got != "invalid" {
		t.Fatalf("invalid role changed to %q", got)
	}
}
