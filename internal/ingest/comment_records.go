package ingest

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

type RawCommentRecord struct {
	ID                 string          `json:"id"`
	HubUserID          string          `json:"hubUserId"`
	AccountID          string          `json:"accountId,omitempty"`
	VideoObjectID      string          `json:"videoObjectId"`
	VideoObjectNonceID string          `json:"videoObjectNonceId,omitempty"`
	VideoTitle         string          `json:"videoTitle,omitempty"`
	ChannelName        string          `json:"channelName,omitempty"`
	ChannelUsername    string          `json:"channelUsername,omitempty"`
	CommentID          string          `json:"commentId"`
	ParentCommentID    string          `json:"parentCommentId,omitempty"`
	AuthorUsername     string          `json:"authorUsername,omitempty"`
	AuthorNickname     string          `json:"authorNickname,omitempty"`
	Content            string          `json:"content"`
	IPRegion           string          `json:"ipRegion,omitempty"`
	CreatedAt          time.Time       `json:"createdAt,omitempty"`
	LikeCount          int             `json:"likeCount"`
	ReplyCount         int             `json:"replyCount"`
	CollectedAt        time.Time       `json:"collectedAt"`
	Source             string          `json:"source"`
	RawPayload         json.RawMessage `json:"rawPayload,omitempty"`
}

type RawCommentQuery struct {
	ObjectID       string
	AccountID      string
	AccountKeyword string
	Source         string
	Keyword        string
	From           time.Time
	To             time.Time
	ExcludeIDs     []string
	Page           int
	PageSize       int
}

type RawCommentPage struct {
	Items      []RawCommentRecord `json:"items"`
	Page       int                `json:"page"`
	PageSize   int                `json:"pageSize"`
	Total      int                `json:"total"`
	TotalPages int                `json:"totalPages"`
}

type VideoCommentSyncState struct {
	HubUserID          string    `json:"hubUserId"`
	AccountID          string    `json:"accountId"`
	VideoObjectID      string    `json:"videoObjectId"`
	VideoObjectNonceID string    `json:"videoObjectNonceId,omitempty"`
	LastCommentCount   int       `json:"lastCommentCount"`
	LastObservedAt     time.Time `json:"lastObservedAt,omitempty"`
	LastCollectedAt    time.Time `json:"lastCollectedAt,omitempty"`
	Status             string    `json:"status"`
	LastError          string    `json:"lastError,omitempty"`
}

func PageRawComments(items []RawCommentRecord, query RawCommentQuery) RawCommentPage {
	query = NormalizeRawCommentQuery(query)
	filtered := make([]RawCommentRecord, 0, len(items))
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	for _, item := range items {
		if query.ObjectID != "" && item.VideoObjectID != query.ObjectID {
			continue
		}
		if query.AccountID != "" && item.AccountID != query.AccountID {
			continue
		}
		if source := strings.TrimSpace(query.Source); source != "" && item.Source != source {
			continue
		}
		if account := strings.ToLower(strings.TrimSpace(query.AccountKeyword)); account != "" &&
			!strings.Contains(strings.ToLower(item.AccountID), account) &&
			!strings.Contains(strings.ToLower(item.ChannelName), account) &&
			!strings.Contains(strings.ToLower(item.ChannelUsername), account) {
			continue
		}
		if len(query.ExcludeIDs) > 0 && containsRawCommentID(query.ExcludeIDs, item.VideoObjectID+"\x00"+item.CommentID) {
			continue
		}
		if !query.From.IsZero() && item.CollectedAt.Before(query.From) {
			continue
		}
		if !query.To.IsZero() && item.CollectedAt.After(query.To) {
			continue
		}
		if keyword != "" {
			matched := false
			for _, value := range []string{item.ID, item.CommentID, item.AuthorUsername, item.AuthorNickname, item.Content, item.VideoTitle, item.ChannelName, item.IPRegion} {
				if strings.Contains(strings.ToLower(value), keyword) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, CloneRawCommentRecord(item))
	}
	filtered = orderRawComments(filtered)
	total := len(filtered)
	totalPages := (total + query.PageSize - 1) / query.PageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if query.Page > totalPages {
		query.Page = totalPages
	}
	start := (query.Page - 1) * query.PageSize
	end := start + query.PageSize
	if end > total {
		end = total
	}
	pageItems := []RawCommentRecord{}
	if start < total {
		pageItems = filtered[start:end]
	}
	return RawCommentPage{Items: pageItems, Page: query.Page, PageSize: query.PageSize, Total: total, TotalPages: totalPages}
}

func containsRawCommentID(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func NormalizeRawCommentQuery(query RawCommentQuery) RawCommentQuery {
	query.Page, query.PageSize = normalizeRawCommentPage(query.Page, query.PageSize)
	return query
}

func orderRawComments(items []RawCommentRecord) []RawCommentRecord {
	if len(items) < 2 {
		return items
	}

	known := make(map[string]struct{}, len(items))
	children := make(map[string][]RawCommentRecord)
	roots := make([]RawCommentRecord, 0, len(items))
	for _, item := range items {
		known[rawCommentKey(item.VideoObjectID, item.CommentID)] = struct{}{}
	}
	for _, item := range items {
		parentID := strings.TrimSpace(item.ParentCommentID)
		if parentID == "" {
			roots = append(roots, item)
			continue
		}
		parentKey := rawCommentKey(item.VideoObjectID, parentID)
		if _, ok := known[parentKey]; !ok {
			roots = append(roots, item)
			continue
		}
		children[parentKey] = append(children[parentKey], item)
	}

	sortRawComments(roots)
	ordered := make([]RawCommentRecord, 0, len(items))
	visited := make(map[string]struct{}, len(items))
	var appendThread func(RawCommentRecord)
	appendThread = func(item RawCommentRecord) {
		key := rawCommentKey(item.VideoObjectID, item.CommentID)
		if _, ok := visited[key]; ok {
			return
		}
		visited[key] = struct{}{}
		ordered = append(ordered, item)
		branch := children[key]
		sortRawComments(branch)
		for _, child := range branch {
			appendThread(child)
		}
	}
	for _, root := range roots {
		appendThread(root)
	}

	// Keep malformed or cyclic records visible without letting them disrupt valid threads.
	if len(ordered) < len(items) {
		remaining := make([]RawCommentRecord, 0, len(items)-len(ordered))
		for _, item := range items {
			if _, ok := visited[rawCommentKey(item.VideoObjectID, item.CommentID)]; !ok {
				remaining = append(remaining, item)
			}
		}
		sortRawComments(remaining)
		ordered = append(ordered, remaining...)
	}
	return ordered
}

func sortRawComments(items []RawCommentRecord) {
	sort.SliceStable(items, func(i, j int) bool {
		leftAt := rawCommentActivityAt(items[i])
		rightAt := rawCommentActivityAt(items[j])
		if !leftAt.Equal(rightAt) {
			return leftAt.After(rightAt)
		}
		if !items[i].CollectedAt.Equal(items[j].CollectedAt) {
			return items[i].CollectedAt.After(items[j].CollectedAt)
		}
		return items[i].ID > items[j].ID
	})
}

func rawCommentActivityAt(item RawCommentRecord) time.Time {
	if !item.CreatedAt.IsZero() {
		return item.CreatedAt
	}
	return item.CollectedAt
}

func rawCommentKey(videoObjectID, commentID string) string {
	return strings.TrimSpace(videoObjectID) + "\x00" + strings.TrimSpace(commentID)
}

func normalizeRawCommentPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize != 20 && pageSize != 50 && pageSize != 100 {
		pageSize = 20
	}
	return page, pageSize
}

const (
	VideoCommentSyncStatusSuccess = "success"
	VideoCommentSyncStatusPartial = "partial"
	VideoCommentSyncStatusSkipped = "skipped"
	VideoCommentSyncStatusFailed  = "failed"
)

func CloneRawCommentRecord(source RawCommentRecord) RawCommentRecord {
	source.RawPayload = append(json.RawMessage(nil), source.RawPayload...)
	return source
}

func CloneRawCommentRecords(items []RawCommentRecord) []RawCommentRecord {
	result := make([]RawCommentRecord, 0, len(items))
	for _, item := range items {
		result = append(result, CloneRawCommentRecord(item))
	}
	return result
}

func FlattenWXCommentExport(hubUserID, accountID string, payload WXCommentExportFile, collectedAt time.Time) []RawCommentRecord {
	hubUserID = strings.TrimSpace(hubUserID)
	accountID = strings.TrimSpace(accountID)
	objectID := strings.TrimSpace(payload.ObjectID)
	if objectID == "" || hubUserID == "" {
		return nil
	}
	if collectedAt.IsZero() {
		collectedAt = time.Now().UTC()
	}
	source := strings.TrimSpace(payload.Source)
	if source == "" {
		source = RunSourceWXCommentExport
	}
	records := make([]RawCommentRecord, 0, len(payload.CommentInfo))
	var appendEntry func(WXCommentEntry, string)
	appendEntry = func(entry WXCommentEntry, parentID string) {
		commentID := strings.TrimSpace(entry.CommentID)
		if commentID == "" {
			commentID = strings.TrimSpace(entry.ReplyCommentID)
		}
		if commentID == "" {
			return
		}
		rawPayload, _ := json.Marshal(entry)
		createdAt := parseWXCommentCreatedAt(entry.Createtime)
		records = append(records, RawCommentRecord{
			ID:                 objectID + "\x00" + commentID,
			HubUserID:          hubUserID,
			AccountID:          accountID,
			VideoObjectID:      objectID,
			VideoObjectNonceID: strings.TrimSpace(payload.ObjectNonceID),
			VideoTitle:         strings.TrimSpace(payload.Title),
			ChannelName:        strings.TrimSpace(payload.Author),
			CommentID:          commentID,
			ParentCommentID:    strings.TrimSpace(parentID),
			AuthorUsername:     strings.TrimSpace(entry.Username),
			AuthorNickname:     strings.TrimSpace(entry.Nickname),
			Content:            strings.TrimSpace(entry.Content),
			IPRegion:           strings.TrimSpace(entry.IPRegionInfo.RegionText),
			CreatedAt:          createdAt,
			LikeCount:          entry.LikeCount,
			ReplyCount:         entry.ExpandCommentCount,
			CollectedAt:        collectedAt,
			Source:             source,
			RawPayload:         rawPayload,
		})
		for _, reply := range entry.LevelTwoComment {
			appendEntry(reply, commentID)
		}
	}
	for _, entry := range payload.CommentInfo {
		appendEntry(entry, "")
	}
	sort.SliceStable(records, func(i, j int) bool {
		leftParent := strings.TrimSpace(records[i].ParentCommentID)
		rightParent := strings.TrimSpace(records[j].ParentCommentID)
		if (leftParent == "") != (rightParent == "") {
			return leftParent == ""
		}
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records
}

func parseWXCommentCreatedAt(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}
	}
	if seconds > 1_000_000_000_000 {
		return time.UnixMilli(seconds).UTC()
	}
	return time.Unix(seconds, 0).UTC()
}
