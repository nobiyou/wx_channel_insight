package ingest

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

type eventIdentifierCandidate struct {
	EventID       string
	ObjectID      string
	ObjectNonceID string
	ChannelName   string
	TitleDense    string
	TitleTokens   []string
	CollectedAt   time.Time
	Complete      bool
}

func backfillEventObjectIdentifiers(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}

	results := append([]Event(nil), events...)
	candidates := buildEventIdentifierCandidates(results)
	if len(candidates) == 0 {
		return results
	}

	for index := range results {
		if hasCompleteEventObjectIdentifiers(results[index]) {
			continue
		}
		candidate, ok := findEventIdentifierCandidate(results[index], candidates)
		if !ok {
			continue
		}
		if strings.TrimSpace(results[index].ObjectID) == "" {
			results[index].ObjectID = candidate.ObjectID
		}
		if strings.TrimSpace(results[index].ObjectNonceID) == "" {
			results[index].ObjectNonceID = candidate.ObjectNonceID
		}
	}

	return results
}

func hasCompleteEventObjectIdentifiers(event Event) bool {
	return strings.TrimSpace(event.ObjectID) != "" && strings.TrimSpace(event.ObjectNonceID) != ""
}

func buildEventIdentifierCandidates(events []Event) []eventIdentifierCandidate {
	candidates := make([]eventIdentifierCandidate, 0, len(events))
	for _, event := range events {
		if strings.TrimSpace(event.EventType) != "video_snapshot" {
			continue
		}
		objectID := strings.TrimSpace(event.ObjectID)
		objectNonceID := strings.TrimSpace(event.ObjectNonceID)
		if objectID == "" && objectNonceID == "" {
			continue
		}
		titleDense := denseEventIdentifierText(event.Title)
		if titleDense == "" {
			continue
		}
		candidates = append(candidates, eventIdentifierCandidate{
			EventID:       strings.TrimSpace(event.ID),
			ObjectID:      objectID,
			ObjectNonceID: objectNonceID,
			ChannelName:   normalizeEventIdentifierText(event.ChannelName),
			TitleDense:    titleDense,
			TitleTokens:   tokenizeEventIdentifierText(event.Title),
			CollectedAt:   event.CollectedAt,
			Complete:      objectID != "" && objectNonceID != "",
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Complete != candidates[j].Complete {
			return candidates[i].Complete
		}
		if candidates[i].CollectedAt.Equal(candidates[j].CollectedAt) {
			return candidates[i].EventID > candidates[j].EventID
		}
		return candidates[i].CollectedAt.After(candidates[j].CollectedAt)
	})

	return candidates
}

func findEventIdentifierCandidate(event Event, candidates []eventIdentifierCandidate) (eventIdentifierCandidate, bool) {
	titleDense := denseEventIdentifierText(event.Title)
	if titleDense == "" {
		return eventIdentifierCandidate{}, false
	}

	channelName := normalizeEventIdentifierText(event.ChannelName)
	titleTokens := tokenizeEventIdentifierText(event.Title)
	if len(titleTokens) == 0 {
		titleTokens = tokenizeEventIdentifierText(event.Keyword)
	}

	for _, candidate := range candidates {
		if candidate.EventID != "" && candidate.EventID == strings.TrimSpace(event.ID) {
			continue
		}
		if channelName != "" && candidate.ChannelName != "" && channelName != candidate.ChannelName {
			continue
		}
		if !titlesProbablyDescribeSameVideo(titleDense, titleTokens, candidate) {
			continue
		}
		return candidate, true
	}

	return eventIdentifierCandidate{}, false
}

func titlesProbablyDescribeSameVideo(titleDense string, titleTokens []string, candidate eventIdentifierCandidate) bool {
	if titleDense == "" || candidate.TitleDense == "" {
		return false
	}
	if titleDense == candidate.TitleDense {
		return true
	}

	minDenseLength := len([]rune(titleDense))
	if candidateLength := len([]rune(candidate.TitleDense)); candidateLength < minDenseLength {
		minDenseLength = candidateLength
	}
	if minDenseLength >= 12 && (strings.Contains(titleDense, candidate.TitleDense) || strings.Contains(candidate.TitleDense, titleDense)) {
		return true
	}

	return countSharedEventIdentifierTokens(titleTokens, candidate.TitleTokens) >= 3
}

func countSharedEventIdentifierTokens(left, right []string) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(left))
	for _, item := range left {
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
	}
	count := 0
	for _, item := range right {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			count++
			delete(seen, item)
		}
	}
	return count
}

func normalizeEventIdentifierText(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func denseEventIdentifierText(value string) string {
	normalized := normalizeEventIdentifierText(value)
	if normalized == "" {
		return ""
	}
	var builder strings.Builder
	for _, char := range normalized {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			builder.WriteRune(unicode.ToLower(char))
		}
	}
	return builder.String()
}

func tokenizeEventIdentifierText(value string) []string {
	value = normalizeEventIdentifierText(value)
	if value == "" {
		return nil
	}

	parts := strings.FieldsFunc(value, func(char rune) bool {
		return !(unicode.IsLetter(char) || unicode.IsNumber(char))
	})
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		results = append(results, part)
	}
	return results
}
