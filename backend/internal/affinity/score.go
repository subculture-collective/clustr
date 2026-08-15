// Package affinity computes deterministic, hub-corrected subreddit affinity.
package affinity

import "math"

type Signal string

const (
	ActiveUsers      Signal = "active_users"
	Authors          Signal = "posting_authors"
	RepeatCoactivity Signal = "repeat_coactivity"
	CrossReferences  Signal = "cross_references"
)

// Weights are configured publication weights. Score renormalizes them over
// layers marked Available so a not-yet-collected signal cannot dilute evidence.
type Weights map[Signal]float64

func DefaultWeights() Weights {
	return Weights{ActiveUsers: .60, Authors: .20, RepeatCoactivity: .10, CrossReferences: .10}
}

// Layer contains symmetric pair evidence and the activity marginals needed for
// positive normalized PMI. Universe is the population used by both marginals.
type Layer struct {
	Observed      float64
	LeftMarginal  float64
	RightMarginal float64
	Universe      float64
	Available     bool
}

type Evidence struct {
	ActiveUsers      Layer
	Authors          Layer
	RepeatCoactivity Layer
	CrossReferences  Layer
}

type Component struct {
	Observed     float64 `json:"observed"`
	PositiveNPMI float64 `json:"positive_npmi"`
	Shrinkage    float64 `json:"shrinkage"`
	Raw          float64 `json:"raw"`
}

type Result struct {
	Affinity         float64              `json:"affinity"`
	RawAffinity      float64              `json:"raw_affinity"`
	ObservedEvidence float64              `json:"observed_evidence"`
	EffectiveWeights map[Signal]float64   `json:"effective_weights"`
	Components       map[Signal]Component `json:"components"`
}

func Score(e Evidence, configured Weights) Result {
	layers := map[Signal]Layer{
		ActiveUsers: e.ActiveUsers, Authors: e.Authors,
		RepeatCoactivity: e.RepeatCoactivity, CrossReferences: e.CrossReferences,
	}
	result := Result{EffectiveWeights: map[Signal]float64{}, Components: map[Signal]Component{}}
	var availableWeight float64
	for signal, layer := range layers {
		weight := configured[signal]
		if layer.Available && weight > 0 && valid(layer) {
			availableWeight += weight
		}
	}
	if availableWeight <= 0 || !finite(availableWeight) {
		return result
	}
	for signal, layer := range layers {
		weight := configured[signal]
		if !layer.Available || weight <= 0 || !valid(layer) {
			result.EffectiveWeights[signal] = 0
			continue
		}
		effective := weight / availableWeight
		component := scoreLayer(layer)
		result.EffectiveWeights[signal] = effective
		result.Components[signal] = component
		result.RawAffinity += effective * component.Raw
		result.ObservedEvidence += layer.Observed
	}
	// The specified evidence product is non-negative but unbounded because of
	// log1p. This monotonic transform publishes a stable normalized affinity.
	result.Affinity = result.RawAffinity / (1 + result.RawAffinity)
	if !finite(result.Affinity) || result.Affinity < 0 {
		result.Affinity, result.RawAffinity = 0, 0
	}
	if result.Affinity > 1 {
		result.Affinity = 1
	}
	return result
}

func valid(layer Layer) bool {
	return finite(layer.Observed) && finite(layer.LeftMarginal) && finite(layer.RightMarginal) && finite(layer.Universe) &&
		layer.Observed > 0 && layer.LeftMarginal > 0 && layer.RightMarginal > 0 && layer.Universe > 0 &&
		layer.Observed <= layer.LeftMarginal && layer.Observed <= layer.RightMarginal &&
		layer.LeftMarginal <= layer.Universe && layer.RightMarginal <= layer.Universe
}

func scoreLayer(layer Layer) Component {
	joint := layer.Observed / layer.Universe
	left := layer.LeftMarginal / layer.Universe
	right := layer.RightMarginal / layer.Universe
	denominator := -math.Log(joint)
	npmi := 0.0
	if denominator > 0 {
		npmi = math.Log(joint/(left*right)) / denominator
	}
	if !finite(npmi) || npmi < 0 {
		npmi = 0
	} else if npmi > 1 {
		npmi = 1
	}
	shrinkage := layer.Observed / (layer.Observed + 5)
	raw := npmi * math.Log1p(layer.Observed) * shrinkage
	return Component{Observed: layer.Observed, PositiveNPMI: npmi, Shrinkage: shrinkage, Raw: raw}
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
