package graph

import "strings"

const AutomationPolicyVersion = "bot-policy-v1"

type AutomationClassification struct {
	Automated bool
	Reason    string
}

type AutomationPolicy struct {
	known map[string]struct{}
}

func NewAutomationPolicy(knownNames []string) AutomationPolicy {
	known := make(map[string]struct{}, len(knownNames)+1)
	known["automoderator"] = struct{}{}
	for _, name := range knownNames {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			known[name] = struct{}{}
		}
	}
	return AutomationPolicy{known: known}
}

func (p AutomationPolicy) Classify(username string, distinctCommunities int, manual *bool) AutomationClassification {
	if manual != nil {
		if *manual {
			return AutomationClassification{Automated: true, Reason: "manual-block"}
		}
		return AutomationClassification{Reason: "manual-allow"}
	}
	name := strings.ToLower(strings.TrimSpace(username))
	if _, found := p.known[name]; found {
		return AutomationClassification{Automated: true, Reason: "known-list"}
	}
	if distinctCommunities >= 20 && strings.HasSuffix(name, "bot") {
		return AutomationClassification{Automated: true, Reason: "broad-bot-name"}
	}
	return AutomationClassification{Reason: "not-automated"}
}
