package ingest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type WXCommentExportFile struct {
	ObjectID             string               `json:"objectId"`
	ObjectNonceID        string               `json:"objectNonceId"`
	Title                string               `json:"title"`
	Author               string               `json:"author"`
	CommentInfo          []WXCommentEntry     `json:"commentInfo"`
	CountInfo            WXCommentExportCount `json:"countInfo"`
	OriginalCommentCount int                  `json:"originalCommentCount"`
	SavedAt              string               `json:"savedAt"`
	Source               string               `json:"source"`
}

type WXCommentExportCount struct {
	CommentCount int `json:"commentCount"`
}

type WXCommentEntry struct {
	Username           string           `json:"username"`
	Nickname           string           `json:"nickname"`
	Content            string           `json:"content"`
	CommentID          string           `json:"commentId"`
	ReplyCommentID     string           `json:"replyCommentId"`
	Createtime         string           `json:"createtime"`
	LikeCount          int              `json:"likeCount"`
	ExpandCommentCount int              `json:"expandCommentCount"`
	LevelTwoComment    []WXCommentEntry `json:"levelTwoComment"`
	IPRegionInfo       struct {
		RegionText string `json:"regionText"`
	} `json:"ipRegionInfo"`
}

func LoadWXCommentExportFile(path string) (WXCommentExportFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WXCommentExportFile{}, err
	}

	var payload WXCommentExportFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return WXCommentExportFile{}, err
	}
	if strings.TrimSpace(payload.ObjectID) == "" {
		return WXCommentExportFile{}, fmt.Errorf("objectId is required")
	}
	return payload, nil
}

func buildSourceBindingTargetFromCommentExport(payload WXCommentExportFile) *SourceBindingTarget {
	commentCount := wxCommentExportTotalCount(payload)
	target := &SourceBindingTarget{
		Title:         strings.TrimSpace(payload.Title),
		ChannelName:   strings.TrimSpace(payload.Author),
		Comment:       deriveRepresentativeComment(payload),
		CommentCount:  commentCount,
		ObjectID:      strings.TrimSpace(payload.ObjectID),
		ObjectNonceID: strings.TrimSpace(payload.ObjectNonceID),
	}
	if savedAt, ok := parseWXCommentExportTime(payload.SavedAt); ok {
		target.PublishedAt = savedAt
	}
	return target
}

func wxCommentExportTotalCount(payload WXCommentExportFile) int {
	topLevelCount := len(payload.CommentInfo)
	replyCount := 0
	for _, item := range payload.CommentInfo {
		replyCount += len(item.LevelTwoComment)
	}
	total := topLevelCount + replyCount
	if payload.OriginalCommentCount > total {
		total = payload.OriginalCommentCount
	}
	if payload.CountInfo.CommentCount > total {
		total = payload.CountInfo.CommentCount
	}
	return total
}

func isWXCommentExportProofPlaceholder(path string, payload WXCommentExportFile) bool {
	if isWXCommentExportProofPlaceholderPath(path) {
		return true
	}

	objectID := strings.ToLower(strings.TrimSpace(payload.ObjectID))
	title := strings.ToLower(strings.TrimSpace(payload.Title))
	return strings.HasPrefix(objectID, "team-runtime-proof-") || strings.Contains(title, "team runtime proof")
}

func isWXCommentExportProofPlaceholderPath(path string) bool {
	normalized := strings.ToLower(normalizeBindingPath(path))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, strings.ToLower(filepath.Join(".tmp-codex", "proof-inputs")))
}

func BuildEventsFromWXCommentExport(hubUserID, machineID string, payload WXCommentExportFile, now time.Time) ([]Event, error) {
	if strings.TrimSpace(payload.ObjectID) == "" {
		return nil, fmt.Errorf("objectId is required")
	}

	collectedAt := now
	if parsed, ok := parseWXCommentExportTime(payload.SavedAt); ok {
		collectedAt = parsed
	}

	replyCount := 0
	for _, item := range payload.CommentInfo {
		replyCount += len(item.LevelTwoComment)
	}
	totalCommentCount := wxCommentExportTotalCount(payload)

	keyword, promptAngle, keywords := deriveWXCommentTopic(payload)
	representative := deriveRepresentativeComment(payload)
	targetAudience := "视频号内容运营"
	if keyword != "" && strings.Contains(keyword, "预算") {
		targetAudience = "本地生活门店老板"
	}

	source := strings.TrimSpace(payload.Source)
	if source == "" {
		source = "wx_channel_comment_export"
	}

	events := []Event{
		{
			ID:            fmt.Sprintf("wx-comment-video-%s", payload.ObjectID),
			HubUserID:     hubUserID,
			MachineID:     machineID,
			EventType:     "video_snapshot",
			Source:        source,
			CollectedAt:   collectedAt,
			ObjectID:      strings.TrimSpace(payload.ObjectID),
			ObjectNonceID: strings.TrimSpace(payload.ObjectNonceID),
			VideoCount:    1,
			CommentCount:  totalCommentCount,
			TopicCount:    maxInt(1, len(keywords)),
			Title:         strings.TrimSpace(payload.Title),
			ChannelName:   strings.TrimSpace(payload.Author),
			Signal:        buildWXVideoSignal(totalCommentCount, replyCount),
			Tags:          append([]string(nil), keywords...),
		},
		{
			ID:                    fmt.Sprintf("wx-comment-comment-%s", payload.ObjectID),
			HubUserID:             hubUserID,
			MachineID:             machineID,
			EventType:             "comment_snapshot",
			Source:                source,
			CollectedAt:           collectedAt.Add(-1 * time.Minute),
			ObjectID:              strings.TrimSpace(payload.ObjectID),
			ObjectNonceID:         strings.TrimSpace(payload.ObjectNonceID),
			CommentCount:          totalCommentCount,
			TopicCount:            maxInt(1, len(keywords)),
			Title:                 "评论区代表性问题",
			Keyword:               keyword,
			Signal:                buildWXCommentSignal(totalCommentCount, replyCount),
			RepresentativeComment: representative,
			TargetAudience:        targetAudience,
			Keywords:              append([]string(nil), keywords...),
		},
	}

	if keyword != "" {
		events = append(events, Event{
			ID:             fmt.Sprintf("wx-comment-topic-%s", payload.ObjectID),
			HubUserID:      hubUserID,
			MachineID:      machineID,
			EventType:      "topic_cluster",
			Source:         source,
			CollectedAt:    collectedAt.Add(-30 * time.Second),
			ObjectID:       strings.TrimSpace(payload.ObjectID),
			ObjectNonceID:  strings.TrimSpace(payload.ObjectNonceID),
			TopicCount:     maxInt(1, len(keywords)),
			Keyword:        keyword,
			Cluster:        deriveWXTopicCluster(keyword),
			Signal:         buildWXTopicSignal(totalCommentCount, replyCount),
			PromptAngle:    promptAngle,
			TargetAudience: targetAudience,
			Keywords:       append([]string(nil), keywords...),
		})
	}

	return events, nil
}

func parseWXCommentExportTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func deriveRepresentativeComment(payload WXCommentExportFile) string {
	best := ""
	bestLikes := -1

	for _, item := range payload.CommentInfo {
		if strings.TrimSpace(item.Content) != "" && item.LikeCount > bestLikes {
			best = strings.TrimSpace(item.Content)
			bestLikes = item.LikeCount
		}
		for _, reply := range item.LevelTwoComment {
			if strings.TrimSpace(reply.Content) != "" && reply.LikeCount > bestLikes {
				best = strings.TrimSpace(reply.Content)
				bestLikes = reply.LikeCount
			}
		}
	}

	if best != "" {
		return best
	}
	if strings.TrimSpace(payload.Title) != "" {
		return fmt.Sprintf("评论区围绕《%s》持续追问具体落地。", strings.TrimSpace(payload.Title))
	}
	return "评论区开始集中追问更具体的执行与落地问题。"
}

func deriveWXCommentTopic(payload WXCommentExportFile) (string, string, []string) {
	tokenScores := map[string]int{}
	accumulate := func(text string, weight int) {
		for _, token := range tokenizeWXText(text) {
			if len([]rune(token)) < 2 {
				continue
			}
			tokenScores[token] += weight
		}
	}

	accumulate(payload.Title, 3)
	for _, item := range payload.CommentInfo {
		accumulate(item.Content, 2+item.LikeCount/5)
		for _, reply := range item.LevelTwoComment {
			accumulate(reply.Content, 1+reply.LikeCount/5)
		}
	}

	type pair struct {
		token string
		score int
	}
	pairs := make([]pair, 0, len(tokenScores))
	for token, score := range tokenScores {
		if score <= 0 {
			continue
		}
		pairs = append(pairs, pair{token: token, score: score})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score == pairs[j].score {
			return pairs[i].token < pairs[j].token
		}
		return pairs[i].score > pairs[j].score
	})

	if len(pairs) == 0 {
		titleKeyword := deriveWXTitleKeyword(payload.Title)
		if titleKeyword == "" {
			return "", "", nil
		}
		return titleKeyword, fmt.Sprintf("围绕评论区高频追问“%s”先做回答型内容，再补执行细节。", titleKeyword), []string{titleKeyword}
	}

	titleKeyword := deriveWXTitleKeyword(payload.Title)
	keywords := make([]string, 0, 3)
	if titleKeyword != "" {
		keywords = append(keywords, titleKeyword)
	}
	for _, item := range pairs {
		if len(keywords) >= 3 {
			break
		}
		if item.token == titleKeyword {
			continue
		}
		keywords = append(keywords, item.token)
	}
	if len(keywords) == 0 {
		return "", "", nil
	}
	keyword := keywords[0]
	promptAngle := fmt.Sprintf("围绕评论区高频追问“%s”先做回答型内容，再补执行细节。", keyword)
	return keyword, promptAngle, keywords
}

func deriveWXTitleKeyword(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if idx := strings.Index(title, "（"); idx > 0 {
		title = strings.TrimSpace(title[:idx])
	}
	if idx := strings.Index(title, "("); idx > 0 {
		title = strings.TrimSpace(title[:idx])
	}
	parts := strings.FieldsFunc(title, func(r rune) bool {
		switch r {
		case '，', '。', '！', '？', '：', '；', ',', '.', '!', '?', ':', ';':
			return true
		default:
			return false
		}
	})
	if len(parts) > 0 {
		title = strings.TrimSpace(parts[0])
	}
	for _, marker := range []string{"为什么", "怎么", "是不是", "到底", "如何", "为何"} {
		if idx := strings.Index(title, marker); idx > 0 {
			title = strings.TrimSpace(title[:idx])
			break
		}
	}
	return strings.TrimSpace(title)
}

func deriveWXTopicCluster(keyword string) string {
	switch {
	case strings.Contains(keyword, "世界模型") || strings.Contains(keyword, "AI"):
		return "AI 认知路线"
	case strings.Contains(keyword, "预算") || strings.Contains(keyword, "团购"):
		return "门店增长"
	case strings.Contains(keyword, "标题") || strings.Contains(keyword, "直播"):
		return "直播复盘模板"
	default:
		return "评论区高追问"
	}
}

func buildWXVideoSignal(totalCommentCount, replyCount int) string {
	switch {
	case replyCount >= 10:
		return "高互动 + 深回复"
	case totalCommentCount >= 50:
		return "评论密度高"
	default:
		return "评论区有追问"
	}
}

func buildWXCommentSignal(totalCommentCount, replyCount int) string {
	switch {
	case replyCount >= 10:
		return "追问密集"
	case totalCommentCount >= 30:
		return "高追问"
	default:
		return "持续讨论"
	}
}

func buildWXTopicSignal(totalCommentCount, replyCount int) string {
	switch {
	case replyCount >= 10:
		return "讨论加深"
	case totalCommentCount >= 30:
		return "持续升温"
	default:
		return "评论成形"
	}
}

func tokenizeWXText(text string) []string {
	replacer := strings.NewReplacer(
		"，", " ", "。", " ", "！", " ", "？", " ", "：", " ", "；", " ",
		",", " ", ".", " ", "!", " ", "?", " ", ":", " ", ";", " ",
		"（", " ", "）", " ", "(", " ", ")", " ", "“", " ", "”", " ", "\"", " ",
		"、", " ", "\n", " ", "\r", " ", "\t", " ",
	)
	normalized := replacer.Replace(strings.TrimSpace(text))
	parts := strings.Fields(normalized)
	result := make([]string, 0, len(parts))
	stopwords := map[string]struct{}{
		"这个": {}, "那个": {}, "什么": {}, "怎么": {}, "是不是": {}, "就是": {}, "可以": {},
		"一下": {}, "一下子": {}, "真的": {}, "现在": {}, "还是": {}, "因为": {}, "所以": {},
		"我们": {}, "你们": {}, "他们": {}, "自己": {}, "一个": {}, "一种": {}, "已经": {},
		"你说": {}, "别只": {}, "能不能": {}, "到底": {}, "到底能": {}, "还有": {},
		"wx_channel": {}, "评论导入": {}, "真实导入": {},
	}
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		if _, blocked := stopwords[token]; blocked {
			continue
		}
		result = append(result, token)
	}
	return result
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
