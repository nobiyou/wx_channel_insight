package ingest

import "strings"

func buildSourceBindingTarget(binding SourceBinding, assessment SourceBindingAssessment, facts EventSummaryFacts) *SourceBindingTarget {
	target := cloneSourceBindingTarget(assessment.Target)
	if target == nil {
		target = &SourceBindingTarget{}
	}

	source := canonicalSource(binding.Source)
	if source == RunSourceWXFeedList {
		target.ChannelUsername = firstNonEmpty(target.ChannelUsername, binding.Username)
	}
	if source == RunSourceWXVideoSearch {
		target.Keyword = firstNonEmpty(target.Keyword, binding.Keyword)
	}
	if source == RunSourceWXManualShare {
		target.ObjectID = firstNonEmpty(target.ObjectID, binding.ObjectID)
		target.ObjectNonceID = firstNonEmpty(target.ObjectNonceID, binding.ObjectNonceID)
	}

	evidenceBinding := binding
	if target != nil {
		evidenceBinding.ObjectID = firstNonEmpty(evidenceBinding.ObjectID, target.ObjectID)
		evidenceBinding.ObjectNonceID = firstNonEmpty(evidenceBinding.ObjectNonceID, target.ObjectNonceID)
	}
	for _, event := range selectSourceBindingEvents(evidenceBinding, facts) {
		mergeSourceBindingTargetEvent(target, event)
	}

	if !sourceBindingTargetHasContent(target) {
		return nil
	}
	return target
}

func selectSourceBindingEvents(binding SourceBinding, facts EventSummaryFacts) []Event {
	source := canonicalSource(binding.Source)
	candidates := make([]Event, 0, len(facts.RecentEvents)+len(facts.Sources))
	seen := map[string]struct{}{}
	appendCandidate := func(event Event) {
		if canonicalSource(event.Source) != source {
			return
		}
		key := strings.TrimSpace(event.ID)
		if key == "" {
			key = strings.TrimSpace(event.ObjectID) + "\x00" + strings.TrimSpace(event.EventType)
		}
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, event)
	}
	for _, event := range facts.RecentEvents {
		appendCandidate(event)
	}
	for _, sourceFacts := range facts.Sources {
		appendCandidate(sourceFacts.LatestEvent)
	}

	matches := func(event Event) bool {
		switch source {
		case RunSourceWXFeedList:
			return strings.TrimSpace(binding.Username) == "" || strings.EqualFold(strings.TrimSpace(event.ChannelUsername), strings.TrimSpace(binding.Username))
		case RunSourceWXVideoSearch:
			return strings.TrimSpace(binding.Keyword) == "" || strings.EqualFold(strings.TrimSpace(event.Keyword), strings.TrimSpace(binding.Keyword))
		case RunSourceWXManualShare:
			if strings.TrimSpace(binding.ObjectID) == "" && strings.TrimSpace(binding.ObjectNonceID) == "" {
				return true
			}
			return sourceBindingEventMatchesObject(event, binding.ObjectID, binding.ObjectNonceID)
		case RunSourceWXCommentExport:
			if strings.TrimSpace(binding.ObjectID) != "" || strings.TrimSpace(binding.ObjectNonceID) != "" {
				return sourceBindingEventMatchesObject(event, binding.ObjectID, binding.ObjectNonceID)
			}
			return true
		default:
			return true
		}
	}

	selected := make([]Event, 0, len(candidates))
	for _, event := range candidates {
		if matches(event) {
			selected = append(selected, event)
		}
	}
	return selected
}

func sourceBindingEventMatchesObject(event Event, objectID, nonceID string) bool {
	objectID = strings.TrimSpace(objectID)
	nonceID = strings.TrimSpace(nonceID)
	return (objectID != "" && (strings.TrimSpace(event.ObjectID) == objectID || strings.TrimSpace(event.ObjectNonceID) == objectID)) ||
		(nonceID != "" && (strings.TrimSpace(event.ObjectID) == nonceID || strings.TrimSpace(event.ObjectNonceID) == nonceID))
}

func mergeSourceBindingTargetEvent(target *SourceBindingTarget, event Event) {
	if target == nil {
		return
	}
	if event.EventType == "video_snapshot" || strings.TrimSpace(target.Title) == "" {
		target.Title = firstNonEmpty(target.Title, event.Title)
	}
	target.ChannelName = firstNonEmpty(target.ChannelName, event.ChannelName)
	target.ChannelUsername = firstNonEmpty(target.ChannelUsername, event.ChannelUsername)
	target.Keyword = firstNonEmpty(target.Keyword, event.Keyword)
	target.Comment = firstNonEmpty(target.Comment, event.RepresentativeComment)
	if target.CommentCount <= 0 && event.CommentCount > 0 {
		target.CommentCount = event.CommentCount
	}
	target.ObjectID = firstNonEmpty(target.ObjectID, event.ObjectID)
	target.ObjectNonceID = firstNonEmpty(target.ObjectNonceID, event.ObjectNonceID)
	if target.PublishedAt.IsZero() && !event.PublishedAt.IsZero() {
		target.PublishedAt = event.PublishedAt
	}
}

func sourceBindingTargetHasContent(target *SourceBindingTarget) bool {
	if target == nil {
		return false
	}
	return firstNonEmpty(
		target.Title,
		target.ChannelName,
		target.ChannelUsername,
		target.Keyword,
		target.Comment,
		target.ObjectID,
		target.ObjectNonceID,
	) != "" || target.CommentCount > 0 || !target.PublishedAt.IsZero()
}
