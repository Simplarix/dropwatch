package domain

import (
	"slices"
	"strings"
)

func NormalizeStatuses(statuses []string) []string {
	result := make([]string, 0, len(statuses))
	seen := make(map[string]struct{}, len(statuses))
	for _, raw := range statuses {
		value := strings.ToLower(strings.TrimSpace(raw))
		value = strings.NewReplacer(" ", "_", "-", "_").Replace(value)
		switch value {
		case "clienthold":
			value = "client_hold"
		case "serverhold":
			value = "server_hold"
		case "redemptionperiod":
			value = "redemption_period"
		case "pendingdelete":
			value = "pending_delete"
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return result
}

func StateFromRDAP(statuses []string, found bool) State {
	if !found {
		return StatePossiblyAvailable
	}
	normalized := NormalizeStatuses(statuses)
	for _, candidate := range []struct {
		status string
		state  State
	}{
		{"pending_delete", StatePendingDelete},
		{"redemption_period", StateRedemptionPeriod},
		{"server_hold", StateServerHold},
		{"client_hold", StateClientHold},
	} {
		if slices.Contains(normalized, candidate.status) {
			return candidate.state
		}
	}
	return StateRegistered
}

func IsAvailabilityState(state State) bool {
	return state == StatePossiblyAvailable || state == StateAvailable
}
