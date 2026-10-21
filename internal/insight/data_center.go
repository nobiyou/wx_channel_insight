package insight

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
)

type DataCenterEntity string

const (
	DataCenterEntityVideos   DataCenterEntity = "videos"
	DataCenterEntityTopics   DataCenterEntity = "topics"
	DataCenterEntityAccounts DataCenterEntity = "accounts"
	DataCenterEntityComments DataCenterEntity = "comments"
)

const (
	dataCenterStatusCurrent  = "current"
	dataCenterStatusArchived = "archived"
)

var (
	ErrDataCenterServiceUnavailable = errors.New("data center service is not configured")
	ErrDataCenterNotFound           = errors.New("data center record was not found")
	ErrDataCenterExclusionStore     = errors.New("data center exclusion store is not configured")
)

type DataCenterDataProvider interface {
	ListEvents(ctx context.Context, query ingest.SummaryQuery) ([]ingest.Event, error)
	ListVideoMetricObservations(ctx context.Context, query ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error)
}

type DataCenterLatestVideoMetricProvider interface {
	ListLatestVideoMetricObservations(ctx context.Context, query ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error)
}

type DataCenterVideoHistoryProvider interface {
	ListVideoHistory(ctx context.Context, query ingest.SummaryQuery, objectID string) ([]ingest.Event, []ingest.VideoMetricObservation, error)
}

type DataCenterAccountProvider interface {
	List(ctx context.Context, workspaceID string) ([]Account, error)
}

type DataCenterRawCommentProvider interface {
	List(ctx context.Context, hubUserID string, query ingest.RawCommentQuery) (ingest.RawCommentPage, error)
}

type DataCenterQuery struct {
	allItems        bool // Internal assessed snapshot; never bound from HTTP input.
	Curation        string
	Performance     string
	MinInteractions *int
	MinLikes        *int
	FreshOnly       bool
	MomentumOnly    bool
	RecentOnly      bool
	HasKeyword      bool
	Refresh         bool
	Tag             string
	PublishedFrom   time.Time
	PublishedTo     time.Time
	CompareBy       string
	HubUserID       string
	DeviceCount     int
	ContextSource   string
	Entity          DataCenterEntity
	Keyword         string
	MonitorKeyword  string
	From            time.Time
	To              time.Time
	Source          string
	Account         string
	Page            int
	PageSize        int
	Sort            string
	Direction       string
	IncludeArchived bool
}

type DataCenterItem struct {
	growthInteractions *int
	Curation           VideoCuration     `json:"curation"`
	Performance        VideoPerformance  `json:"performance"`
	ID                 string            `json:"id"`
	Entity             DataCenterEntity  `json:"entity"`
	Status             string            `json:"status"`
	HistoryAvailable   bool              `json:"historyAvailable"`
	DeletedAt          time.Time         `json:"deletedAt,omitempty"`
	DeletedBy          string            `json:"deletedBy,omitempty"`
	Source             string            `json:"source,omitempty"`
	Sources            []string          `json:"sources,omitempty"`
	SourceKeywords     []string          `json:"sourceKeywords"`
	ObservedAt         time.Time         `json:"observedAt,omitempty"`
	PublishedAt        time.Time         `json:"publishedAt,omitempty"`
	ObjectID           string            `json:"objectId,omitempty"`
	ObjectNonceID      string            `json:"objectNonceId,omitempty"`
	CoverURL           string            `json:"coverUrl,omitempty"`
	Title              string            `json:"title,omitempty"`
	ChannelName        string            `json:"channelName,omitempty"`
	ChannelUsername    string            `json:"channelUsername,omitempty"`
	AccountID          string            `json:"accountId,omitempty"`
	IdentityStatus     string            `json:"identityStatus,omitempty"`
	Region             string            `json:"region,omitempty"`
	Industry           string            `json:"industry,omitempty"`
	Note               string            `json:"note,omitempty"`
	ProfileSource      string            `json:"profileSource,omitempty"`
	IsPriority         bool              `json:"isPriority,omitempty"`
	Keyword            string            `json:"keyword,omitempty"`
	Cluster            string            `json:"cluster,omitempty"`
	Theme              string            `json:"theme,omitempty"`
	Representative     string            `json:"representative,omitempty"`
	Sentiment          string            `json:"sentiment,omitempty"`
	ActionSuggestion   string            `json:"actionSuggestion,omitempty"`
	Signal             string            `json:"signal,omitempty"`
	SourceAccounts     []string          `json:"sourceAccounts,omitempty"`
	RelatedKeywords    []string          `json:"relatedKeywords,omitempty"`
	Tags               []string          `json:"tags,omitempty"`
	Keywords           []string          `json:"keywords,omitempty"`
	Views              *int              `json:"views,omitempty"`
	Likes              *int              `json:"likes,omitempty"`
	Comments           *int              `json:"comments,omitempty"`
	Favorites          *int              `json:"favorites,omitempty"`
	Forwards           *int              `json:"forwards,omitempty"`
	VideoCount         *int              `json:"videoCount,omitempty"`
	Mentions           *int              `json:"mentions,omitempty"`
	RelatedVideos      *int              `json:"relatedVideos,omitempty"`
	QuestionRate       *int              `json:"questionRate,omitempty"`
	CommentID          string            `json:"commentId,omitempty"`
	ParentCommentID    string            `json:"parentCommentId,omitempty"`
	AuthorUsername     string            `json:"authorUsername,omitempty"`
	AuthorNickname     string            `json:"authorNickname,omitempty"`
	Content            string            `json:"content,omitempty"`
	IPRegion           string            `json:"ipRegion,omitempty"`
	CommentCreatedAt   time.Time         `json:"commentCreatedAt,omitempty"`
	CommentLikeCount   *int              `json:"commentLikeCount,omitempty"`
	ReplyCount         *int              `json:"replyCount,omitempty"`
	ObservationCount   int               `json:"observationCount,omitempty"`
	GrowthRate         *float64          `json:"growthRate"`
	GrowthStatus       string            `json:"growthStatus,omitempty"`
	GrowthBasis        string            `json:"growthBasis,omitempty"`
	GrowthWindow       VideoGrowthWindow `json:"growthWindow,omitempty"`
	DataBasis          string            `json:"dataBasis,omitempty"`
}

type DataCenterHistoryItem struct {
	ID              string    `json:"id"`
	EntityID        string    `json:"entityId"`
	ObservedAt      time.Time `json:"observedAt"`
	Source          string    `json:"source,omitempty"`
	EvidenceType    string    `json:"evidenceType"`
	Title           string    `json:"title,omitempty"`
	ChannelName     string    `json:"channelName,omitempty"`
	ChannelUsername string    `json:"channelUsername,omitempty"`
	Keyword         string    `json:"keyword,omitempty"`
	Theme           string    `json:"theme,omitempty"`
	Representative  string    `json:"representative,omitempty"`
	Signal          string    `json:"signal,omitempty"`
	Views           *int      `json:"views,omitempty"`
	Likes           *int      `json:"likes,omitempty"`
	Comments        *int      `json:"comments,omitempty"`
	Favorites       *int      `json:"favorites,omitempty"`
	Forwards        *int      `json:"forwards,omitempty"`
	VideoCount      *int      `json:"videoCount,omitempty"`
	Mentions        *int      `json:"mentions,omitempty"`
	RelatedVideos   *int      `json:"relatedVideos,omitempty"`
	QuestionRate    *int      `json:"questionRate,omitempty"`
}

type DataCenterPage struct {
	Management       *VideoManagementSummary  `json:"management,omitempty"`
	Analysis         *VideoOperationsAnalysis `json:"analysis,omitempty"`
	Entity           DataCenterEntity         `json:"entity"`
	Items            []DataCenterItem         `json:"items"`
	Page             int                      `json:"page"`
	PageSize         int                      `json:"pageSize"`
	Total            int                      `json:"total"`
	TotalPages       int                      `json:"totalPages"`
	Summary          DataCenterSummary        `json:"summary"`
	AsOf             string                   `json:"asOf"`
	GeneratedAt      time.Time                `json:"generatedAt"`
	HistoryAvailable bool                     `json:"historyAvailable"`
	ViewStatus       DataCenterViewStatus     `json:"viewStatus"`
}

type DataCenterSummary struct {
	Videos   int `json:"videos"`
	Topics   int `json:"topics"`
	Accounts int `json:"accounts"`
	Comments int `json:"comments"`
}

type DataCenterViewStatus struct {
	State    string `json:"state"`
	Source   string `json:"source"`
	Evidence string `json:"evidence"`
	Notice   string `json:"notice,omitempty"`
}

type DataCenterHistoryPage struct {
	Entity           DataCenterEntity        `json:"entity"`
	EntityID         string                  `json:"entityId"`
	Items            []DataCenterHistoryItem `json:"items"`
	Page             int                     `json:"page"`
	PageSize         int                     `json:"pageSize"`
	Total            int                     `json:"total"`
	TotalPages       int                     `json:"totalPages"`
	HistoryAvailable bool                    `json:"historyAvailable"`
	Evidence         string                  `json:"evidence"`
}

type DataCenterMutationResult struct {
	Entity         DataCenterEntity `json:"entity"`
	EntityID       string           `json:"entityId"`
	DeletedAt      time.Time        `json:"deletedAt,omitempty"`
	DeletedBy      string           `json:"deletedBy,omitempty"`
	DeletionMode   string           `json:"deletionMode"`
	ImpactScope    []string         `json:"impactScope"`
	ReadModelState string           `json:"readModelState"`
	Restored       bool             `json:"restored"`
}

type DataCenterServiceConfig struct {
	VideoEvidenceTTL         time.Duration
	CurationRepository       VideoCurationRepository
	DataProvider             DataCenterDataProvider
	AccountProvider          DataCenterAccountProvider
	RawCommentProvider       DataCenterRawCommentProvider
	WorkspaceContextResolver WorkspaceContextResolver
	ExclusionRepository      DataCenterExclusionRepository
	Now                      func() time.Time
}

type DataCenterService struct {
	videoEvidenceMu    sync.Mutex
	videoEvidence      *dataCenterVideoEvidence
	videoEvidenceTTL   time.Duration
	curationMu         sync.Mutex
	curations          VideoCurationRepository
	dataProvider       DataCenterDataProvider
	accountProvider    DataCenterAccountProvider
	rawCommentProvider DataCenterRawCommentProvider
	contextResolver    WorkspaceContextResolver
	exclusions         DataCenterExclusionRepository
	now                func() time.Time
}

func NewDataCenterService(config DataCenterServiceConfig) *DataCenterService {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &DataCenterService{
		videoEvidenceTTL:   config.VideoEvidenceTTL,
		curations:          config.CurationRepository,
		dataProvider:       config.DataProvider,
		accountProvider:    config.AccountProvider,
		rawCommentProvider: config.RawCommentProvider,
		contextResolver:    config.WorkspaceContextResolver,
		exclusions:         config.ExclusionRepository,
		now:                now,
	}
}

func NormalizeDataCenterEntity(value string) (DataCenterEntity, bool) {
	switch DataCenterEntity(strings.ToLower(strings.TrimSpace(value))) {
	case DataCenterEntityVideos:
		return DataCenterEntityVideos, true
	case DataCenterEntityTopics:
		return DataCenterEntityTopics, true
	case DataCenterEntityAccounts:
		return DataCenterEntityAccounts, true
	case DataCenterEntityComments:
		return DataCenterEntityComments, true
	default:
		return "", false
	}
}

func (s *DataCenterService) Query(ctx context.Context, query DataCenterQuery) (DataCenterPage, error) {
	query, err := normalizeDataCenterQuery(query)
	if err != nil {
		return DataCenterPage{}, err
	}
	workspaceID, hubUserID, err := s.resolveContext(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return DataCenterPage{}, err
	}
	if query.Entity == DataCenterEntityComments && s.rawCommentProvider != nil &&
		query.Sort == "observed_at" && query.Direction == "desc" {
		return s.queryRawComments(ctx, workspaceID, hubUserID, query)
	}
	// Video rows need append-only evidence for collection provenance and the
	// growth window, while still exposing one latest snapshot per identity.
	var rows map[DataCenterEntity][]DataCenterItem
	var histories map[string][]DataCenterHistoryItem
	var discoveries map[string][]time.Time
	if query.Entity == DataCenterEntityVideos {
		var evidence *dataCenterVideoEvidence
		evidence, err = s.videoEvidenceRows(ctx, workspaceID, hubUserID, query.From, query.To, query.Refresh)
		if err == nil {
			rows = map[DataCenterEntity][]DataCenterItem{DataCenterEntityVideos: evidence.rows}
			discoveries = evidence.discoveries
		}
	} else {
		rows, histories, err = s.buildRowsWithoutRawComments(ctx, workspaceID, hubUserID)
	}
	if err != nil {
		return DataCenterPage{}, err
	}
	exclusions, err := s.exclusionMap(ctx, workspaceID)
	if err != nil {
		return DataCenterPage{}, err
	}

	curations := map[string]VideoCuration{}
	if query.Entity == DataCenterEntityVideos {
		curations, err = s.curationMap(ctx, workspaceID)
		if err != nil {
			return DataCenterPage{}, err
		}
	}
	filtered := make([]DataCenterItem, 0, len(rows))
	videoCandidates := []DataCenterItem{}
	for _, row := range rows[query.Entity] {
		if exclusion, ok := exclusions[exclusionKey(query.Entity, row.ID)]; ok {
			row.Status = dataCenterStatusArchived
			row.DeletedAt = exclusion.DeletedAt
			row.DeletedBy = exclusion.DeletedBy
			if !query.IncludeArchived {
				continue
			}
		} else {
			row.Status = dataCenterStatusCurrent
			if query.IncludeArchived && row.Status == dataCenterStatusArchived {
				continue
			}
		}
		if query.Entity != DataCenterEntityVideos {
			row.HistoryAvailable = len(histories[row.ID]) > 0
		}
		if query.Entity == DataCenterEntityVideos {
			row.Curation = curations[row.ID]
			if row.Curation.Status == "" {
				row.Curation = VideoCuration{VideoID: row.ID, Status: "unreviewed", Tags: []string{}}
			}
			videoCandidates = append(videoCandidates, row)
			continue
		}
		if !matchesDataCenterQuery(row, query) {
			continue
		}
		filtered = append(filtered, row)
	}

	var management *VideoManagementSummary
	var recentHot VideoAssessmentStatus
	if query.Entity == DataCenterEntityVideos {
		reference := s.now()
		if !query.To.IsZero() && query.To.Before(reference) {
			reference = query.To
		}
		assessVideoPerformance(videoCandidates, reference)
		for _, row := range videoCandidates {
			if matchesDataCenterQuery(row, query) {
				filtered = append(filtered, row)
			}
		}
		recentHot = recentHotReadiness(filtered)
		filtered, management = filterVideoWorkbench(filtered, query)
	}
	sortDataCenterItems(filtered, query.Sort, query.Direction)
	page, pageSize, totalPages := paginateDataCenter(len(filtered), query.Page, query.PageSize)
	start := (page - 1) * pageSize
	end := minDataCenterInt(start+pageSize, len(filtered))
	items := []DataCenterItem{}
	if start < len(filtered) {
		items = cloneDataCenterItems(filtered[start:end])
	}
	if query.allItems {
		items = cloneDataCenterItems(filtered)
	}
	summary := summarizeDataCenterRows(rows, exclusions)
	// The active collection count describes all matching rows, before paging.
	switch query.Entity {
	case DataCenterEntityVideos:
		summary = DataCenterSummary{Videos: len(filtered)}
	case DataCenterEntityTopics:
		summary.Topics = len(filtered)
	case DataCenterEntityAccounts:
		summary.Accounts = len(filtered)
	case DataCenterEntityComments:
		summary.Comments = len(filtered)
	}
	analysis := buildVideoOperationsAnalysis(filtered, discoveries, query)
	if analysis != nil {
		analysis.RecentHot = recentHot
	}
	return DataCenterPage{
		Management:       management,
		Entity:           query.Entity,
		Analysis:         analysis,
		Items:            items,
		Page:             page,
		PageSize:         pageSize,
		Total:            len(filtered),
		TotalPages:       totalPages,
		Summary:          summary,
		AsOf:             "latest",
		GeneratedAt:      s.now().UTC(),
		HistoryAvailable: len(histories) > 0 || len(discoveries) > 0,
		ViewStatus: DataCenterViewStatus{
			State:    "available",
			Source:   "insight_ingest_events, insight_video_metric_observations",
			Evidence: "当前记录按实体身份去重，保留最新真实采集证据。",
		},
	}, nil
}

func (s *DataCenterService) queryRawComments(ctx context.Context, workspaceID, hubUserID string, query DataCenterQuery) (DataCenterPage, error) {
	rows, histories, err := s.buildRowsWithoutRawComments(ctx, workspaceID, hubUserID)
	if err != nil {
		return DataCenterPage{}, err
	}
	exclusions, err := s.exclusionMap(ctx, workspaceID)
	if err != nil {
		return DataCenterPage{}, err
	}
	excludedIDs := rawCommentExclusionIDs(exclusions)
	pageQuery := ingest.RawCommentQuery{
		AccountKeyword: query.Account,
		Source:         query.Source,
		Keyword:        query.Keyword,
		From:           query.From,
		To:             query.To,
		Page:           query.Page,
		PageSize:       query.PageSize,
	}
	if !query.IncludeArchived {
		pageQuery.ExcludeIDs = excludedIDs
	}
	page, err := s.rawCommentProvider.List(ctx, hubUserID, pageQuery)
	if err != nil {
		return DataCenterPage{}, err
	}
	items := make([]DataCenterItem, 0, len(page.Items))
	for _, record := range page.Items {
		item := itemFromRawComment(record)
		if item.ID == "" {
			continue
		}
		if exclusion, ok := exclusions[exclusionKey(DataCenterEntityComments, item.ID)]; ok {
			item.Status = dataCenterStatusArchived
			item.DeletedAt = exclusion.DeletedAt
			item.DeletedBy = exclusion.DeletedBy
			if !query.IncludeArchived {
				continue
			}
		} else {
			item.Status = dataCenterStatusCurrent
		}
		item.HistoryAvailable = true
		items = append(items, item)
	}
	summaryQuery := ingest.RawCommentQuery{ExcludeIDs: excludedIDs, Page: 1, PageSize: 20}
	currentComments, err := s.rawCommentProvider.List(ctx, hubUserID, summaryQuery)
	if err != nil {
		return DataCenterPage{}, err
	}
	summary := summarizeDataCenterRows(rows, exclusions)
	summary.Comments = currentComments.Total
	return DataCenterPage{
		Entity:           DataCenterEntityComments,
		Items:            cloneDataCenterItems(items),
		Page:             page.Page,
		PageSize:         page.PageSize,
		Total:            page.Total,
		TotalPages:       page.TotalPages,
		Summary:          summary,
		AsOf:             "latest",
		GeneratedAt:      s.now().UTC(),
		HistoryAvailable: currentComments.Total > 0 || len(histories) > 0,
		ViewStatus: DataCenterViewStatus{
			State:    "available",
			Source:   "insight_comment_records, insight_ingest_events, insight_video_metric_observations",
			Evidence: "评论库直接按数据库条件筛选、排序和分页；评论洞察仍是独立聚合数据。",
		},
	}, nil
}

func rawCommentExclusionIDs(exclusions map[string]DataCenterExclusion) []string {
	prefix := string(DataCenterEntityComments) + "\x00"
	ids := make([]string, 0)
	for key := range exclusions {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		id := strings.TrimPrefix(key, prefix)
		id = strings.TrimPrefix(id, "comment:")
		if parts := strings.SplitN(id, "\x00", 2); len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// ListAccountRows returns the current account rows without applying the
// data-center pagination limit. Account APIs use this single snapshot to
// enrich a paginated account-master response with derived metrics.
func (s *DataCenterService) ListAccountRows(ctx context.Context, query DataCenterQuery) ([]DataCenterItem, error) {
	query.Entity = DataCenterEntityAccounts
	query.Page = 1
	query.PageSize = 20
	query, err := normalizeDataCenterQuery(query)
	if err != nil {
		return nil, err
	}
	workspaceID, hubUserID, err := s.resolveContext(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return nil, err
	}
	// Account APIs only need video-derived account metrics. Avoid scanning the
	// full raw comment archive here; the data-center comments view still uses
	// the complete buildRows path below.
	rows, _, err := s.buildRowsWithoutRawComments(ctx, workspaceID, hubUserID)
	if err != nil {
		return nil, err
	}
	exclusions, err := s.exclusionMap(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	filtered := make([]DataCenterItem, 0, len(rows[DataCenterEntityAccounts]))
	for _, row := range rows[DataCenterEntityAccounts] {
		if exclusion, ok := exclusions[exclusionKey(DataCenterEntityAccounts, row.ID)]; ok {
			row.Status = dataCenterStatusArchived
			row.DeletedAt = exclusion.DeletedAt
			row.DeletedBy = exclusion.DeletedBy
			if !query.IncludeArchived {
				continue
			}
		} else {
			row.Status = dataCenterStatusCurrent
		}
		if matchesDataCenterQuery(row, query) {
			filtered = append(filtered, row)
		}
	}
	sortDataCenterItems(filtered, query.Sort, query.Direction)
	return cloneDataCenterItems(filtered), nil
}

func summarizeDataCenterRows(rows map[DataCenterEntity][]DataCenterItem, exclusions map[string]DataCenterExclusion) DataCenterSummary {
	currentCount := func(entity DataCenterEntity) int {
		count := 0
		for _, row := range rows[entity] {
			if _, archived := exclusions[exclusionKey(entity, row.ID)]; archived {
				continue
			}
			count++
		}
		return count
	}

	return DataCenterSummary{
		Videos:   currentCount(DataCenterEntityVideos),
		Topics:   currentCount(DataCenterEntityTopics),
		Accounts: currentCount(DataCenterEntityAccounts),
		Comments: currentCount(DataCenterEntityComments),
	}
}

func (s *DataCenterService) History(ctx context.Context, query DataCenterQuery, entityID string) (DataCenterHistoryPage, error) {
	query, err := normalizeDataCenterQuery(query)
	if err != nil {
		return DataCenterHistoryPage{}, err
	}
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return DataCenterHistoryPage{}, fmt.Errorf("%w: empty entity id", ErrDataCenterNotFound)
	}
	workspaceID, hubUserID, err := s.resolveContext(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return DataCenterHistoryPage{}, err
	}
	if query.Entity == DataCenterEntityVideos {
		if provider, ok := s.dataProvider.(DataCenterVideoHistoryProvider); ok {
			objectID := strings.TrimPrefix(entityID, "video:")
			events, observations, err := provider.ListVideoHistory(ctx, ingest.SummaryQuery{HubUserID: hubUserID}, objectID)
			if err != nil {
				return DataCenterHistoryPage{}, err
			}
			items := make([]DataCenterHistoryItem, 0, len(events)+len(observations))
			for _, event := range events {
				item := eventItemFromIngestEvent(event)
				if item.ID == entityID {
					items = append(items, historyFromItem(item, "event", event.ID))
				}
			}
			for _, observation := range observations {
				item := itemFromVideoObservation(observation)
				if item.ID == entityID {
					items = append(items, historyFromItem(item, "metric_observation", observation.ID))
				}
			}
			return paginateDataCenterHistory(query, entityID, dedupeDataCenterHistory(items))
		}
	}
	rows, histories, err := s.buildRows(ctx, workspaceID, hubUserID)
	if err != nil {
		return DataCenterHistoryPage{}, err
	}
	found := false
	for _, row := range rows[query.Entity] {
		if row.ID == entityID {
			found = true
			break
		}
	}
	if !found {
		return DataCenterHistoryPage{}, ErrDataCenterNotFound
	}
	items := append([]DataCenterHistoryItem(nil), histories[entityID]...)
	return paginateDataCenterHistory(query, entityID, items)
}

func paginateDataCenterHistory(query DataCenterQuery, entityID string, items []DataCenterHistoryItem) (DataCenterHistoryPage, error) {
	filtered := items[:0]
	for _, item := range items {
		if !query.From.IsZero() && item.ObservedAt.Before(query.From) {
			continue
		}
		if !query.To.IsZero() && item.ObservedAt.After(query.To) {
			continue
		}
		if query.Source != "" && !strings.EqualFold(item.Source, query.Source) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].ObservedAt.After(filtered[j].ObservedAt) })
	page, pageSize, totalPages := paginateDataCenter(len(filtered), query.Page, query.PageSize)
	start := (page - 1) * pageSize
	end := minDataCenterInt(start+pageSize, len(filtered))
	pageItems := []DataCenterHistoryItem{}
	if start < len(filtered) {
		pageItems = append(pageItems, filtered[start:end]...)
	}
	return DataCenterHistoryPage{
		Entity:           query.Entity,
		EntityID:         entityID,
		Items:            pageItems,
		Page:             page,
		PageSize:         pageSize,
		Total:            len(filtered),
		TotalPages:       totalPages,
		HistoryAvailable: len(filtered) > 0,
		Evidence:         "历史仅展示实际事件或指标观察，不补造缺失记录。",
	}, nil
}

func (s *DataCenterService) SoftDelete(ctx context.Context, query DataCenterQuery, entityID, actor string) (DataCenterMutationResult, error) {
	entity, ok := NormalizeDataCenterEntity(string(query.Entity))
	if !ok {
		return DataCenterMutationResult{}, fmt.Errorf("unsupported data center entity %q", query.Entity)
	}
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return DataCenterMutationResult{}, ErrDataCenterNotFound
	}
	if s.exclusions == nil {
		return DataCenterMutationResult{}, ErrDataCenterExclusionStore
	}
	_, _, err := s.findRow(ctx, query, entity, entityID)
	if err != nil {
		return DataCenterMutationResult{}, err
	}
	workspaceID, _, err := s.resolveContext(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return DataCenterMutationResult{}, err
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "local-operator"
	}
	exclusion, err := s.exclusions.Upsert(ctx, DataCenterExclusion{
		WorkspaceID: workspaceID,
		Entity:      entity,
		EntityID:    entityID,
		DeletedAt:   s.now().UTC(),
		DeletedBy:   actor,
	})
	if err != nil {
		return DataCenterMutationResult{}, err
	}
	return DataCenterMutationResult{
		Entity:         entity,
		EntityID:       entityID,
		DeletedAt:      exclusion.DeletedAt,
		DeletedBy:      exclusion.DeletedBy,
		DeletionMode:   "soft",
		ImpactScope:    []string{"data-center", "analysis-rankings"},
		ReadModelState: "filtered-on-read",
	}, nil
}

func (s *DataCenterService) Restore(ctx context.Context, query DataCenterQuery, entityID string) (DataCenterMutationResult, error) {
	entity, ok := NormalizeDataCenterEntity(string(query.Entity))
	if !ok {
		return DataCenterMutationResult{}, fmt.Errorf("unsupported data center entity %q", query.Entity)
	}
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return DataCenterMutationResult{}, ErrDataCenterNotFound
	}
	if s.exclusions == nil {
		return DataCenterMutationResult{}, ErrDataCenterExclusionStore
	}
	workspaceID, _, err := s.resolveContext(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return DataCenterMutationResult{}, err
	}
	found, err := s.exclusions.Restore(ctx, workspaceID, entity, entityID)
	if err != nil {
		return DataCenterMutationResult{}, err
	}
	if !found {
		return DataCenterMutationResult{}, ErrDataCenterNotFound
	}
	return DataCenterMutationResult{
		Entity:         entity,
		EntityID:       entityID,
		DeletionMode:   "soft",
		ImpactScope:    []string{"data-center", "analysis-rankings"},
		ReadModelState: "restored-on-read",
		Restored:       true,
	}, nil
}

func (s *DataCenterService) findRow(ctx context.Context, query DataCenterQuery, entity DataCenterEntity, entityID string) (DataCenterItem, map[string][]DataCenterHistoryItem, error) {
	workspaceID, hubUserID, err := s.resolveContext(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return DataCenterItem{}, nil, err
	}
	rows, histories, err := s.buildRows(ctx, workspaceID, hubUserID)
	if err != nil {
		return DataCenterItem{}, nil, err
	}
	for _, row := range rows[entity] {
		if row.ID == entityID {
			return row, histories, nil
		}
	}
	return DataCenterItem{}, nil, ErrDataCenterNotFound
}

func (s *DataCenterService) resolveContext(ctx context.Context, hubUserID string, deviceCount int, contextSource string) (string, string, error) {
	if s.contextResolver == nil {
		return "", strings.TrimSpace(hubUserID), nil
	}
	resolved, err := s.contextResolver.Resolve(ctx, workspace.ContextQuery{
		HubUserID:     hubUserID,
		DeviceCount:   deviceCount,
		ContextSource: contextSource,
	})
	if err != nil {
		return "", "", err
	}
	return resolved.Workspace.ID, resolved.Workspace.OwnerUserID, nil
}

func (s *DataCenterService) exclusionMap(ctx context.Context, workspaceID string) (map[string]DataCenterExclusion, error) {
	result := map[string]DataCenterExclusion{}
	if s.exclusions == nil {
		return result, nil
	}
	items, err := s.exclusions.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		result[exclusionKey(item.Entity, item.EntityID)] = item
	}
	return result, nil
}

func exclusionKey(entity DataCenterEntity, entityID string) string {
	return string(entity) + "\x00" + strings.TrimSpace(entityID)
}

func normalizeDataCenterQuery(query DataCenterQuery) (DataCenterQuery, error) {
	entity, ok := NormalizeDataCenterEntity(string(query.Entity))
	if !ok {
		return DataCenterQuery{}, fmt.Errorf("unsupported data center entity %q", query.Entity)
	}
	query.Entity = entity
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Page < 1 {
		return DataCenterQuery{}, errors.New("data center page must be at least 1")
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.PageSize != 20 && query.PageSize != 50 {
		return DataCenterQuery{}, errors.New("data center pageSize must be 20 or 50")
	}
	query.Sort = strings.ToLower(strings.TrimSpace(query.Sort))
	if query.Sort == "" {
		query.Sort = "observed_at"
	}
	query.Direction = strings.ToLower(strings.TrimSpace(query.Direction))
	if query.Direction == "" {
		query.Direction = "desc"
	}
	if query.Direction != "asc" && query.Direction != "desc" {
		return DataCenterQuery{}, errors.New("data center direction must be asc or desc")
	}
	if !dataCenterSortAllowed(query.Entity, query.Sort) {
		return DataCenterQuery{}, fmt.Errorf("sort %q is not supported for %s", query.Sort, query.Entity)
	}
	if !query.From.IsZero() && !query.To.IsZero() && query.To.Before(query.From) {
		return DataCenterQuery{}, errors.New("data center to must not be before from")
	}
	if !query.PublishedFrom.IsZero() && !query.PublishedTo.IsZero() && query.PublishedTo.Before(query.PublishedFrom) {
		return DataCenterQuery{}, errors.New("publishedTo must not be before publishedFrom")
	}
	query.CompareBy = strings.ToLower(strings.TrimSpace(query.CompareBy))
	if query.CompareBy != "" && query.CompareBy != "account" && query.CompareBy != "keyword" {
		return DataCenterQuery{}, errors.New("compareBy must be account or keyword")
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.MonitorKeyword = strings.TrimSpace(query.MonitorKeyword)
	query.Source = strings.TrimSpace(query.Source)
	query.Account = strings.TrimSpace(query.Account)
	if query.Curation != "" && query.Curation != "all" && query.Curation != "active" && !validVideoCurationStatus(query.Curation) {
		return DataCenterQuery{}, errors.New("unsupported curation filter")
	}
	if query.Performance != "" && query.Performance != "standout" && query.Performance != "typical" && query.Performance != "underperforming" && query.Performance != "insufficient_data" {
		return DataCenterQuery{}, errors.New("unsupported performance filter")
	}
	if query.MinInteractions != nil && *query.MinInteractions < 0 || query.MinLikes != nil && *query.MinLikes < 0 {
		return DataCenterQuery{}, errors.New("minimum metrics must not be negative")
	}
	query.Tag = strings.TrimSpace(query.Tag)
	return query, nil
}

func dataCenterSortAllowed(entity DataCenterEntity, sortKey string) bool {
	allowed := map[DataCenterEntity]map[string]bool{
		DataCenterEntityVideos:   {"observed_at": true, "published_at": true, "likes": true, "comments": true, "favorites": true, "forwards": true, "growth_rate": true, "interactions": true, "growth_per_hour": true, "relative_performance": true, "recent_heat": true},
		DataCenterEntityTopics:   {"observed_at": true, "related_videos": true, "comments": true, "growth_rate": true},
		DataCenterEntityAccounts: {"observed_at": true, "video_count": true, "likes": true, "comments": true, "favorites": true, "forwards": true},
		DataCenterEntityComments: {"observed_at": true, "question_rate": true, "sentiment": true},
	}
	return allowed[entity][sortKey]
}

func matchesDataCenterQuery(item DataCenterItem, query DataCenterQuery) bool {
	if query.HasKeyword && len(item.SourceKeywords) == 0 {
		return false
	}
	if !query.PublishedFrom.IsZero() && (item.PublishedAt.IsZero() || item.PublishedAt.Before(query.PublishedFrom)) {
		return false
	}
	if !query.PublishedTo.IsZero() && (item.PublishedAt.IsZero() || item.PublishedAt.After(query.PublishedTo)) {
		return false
	}
	if query.MonitorKeyword != "" && !containsExact(item.SourceKeywords, query.MonitorKeyword) {
		return false
	}
	if !query.From.IsZero() && item.ObservedAt.Before(query.From) {
		return false
	}
	if !query.To.IsZero() && item.ObservedAt.After(query.To) {
		return false
	}
	if query.Source != "" && !strings.EqualFold(query.Source, item.Source) && !containsFold(item.Sources, query.Source) {
		return false
	}
	if query.Account != "" {
		if !containsFold([]string{item.ChannelName, item.ChannelUsername}, query.Account) && !containsFold(item.SourceAccounts, query.Account) {
			return false
		}
	}
	if query.Keyword == "" {
		return true
	}
	needle := strings.ToLower(query.Keyword)
	fields := []string{
		item.Curation.Note,
		item.ID,
		item.Title,
		item.ChannelName,
		item.ChannelUsername,
		item.Keyword,
		item.Cluster,
		item.Theme,
		item.Representative,
		item.Sentiment,
		item.ActionSuggestion,
		item.Source,
		item.CommentID,
		item.AuthorUsername,
		item.AuthorNickname,
		item.Content,
		item.IPRegion,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return containsFold(item.Curation.Tags, query.Keyword) || containsFold(item.Tags, query.Keyword) || containsFold(item.Keywords, query.Keyword) || containsFold(item.RelatedKeywords, query.Keyword)
}

func sortDataCenterItems(items []DataCenterItem, sortKey, direction string) {
	sort.SliceStable(items, func(i, j int) bool {
		cmp := compareDataCenterItems(items[i], items[j], sortKey)
		if direction == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})
}

func compareDataCenterItems(left, right DataCenterItem, sortKey string) int {
	if sortKey == "sentiment" {
		return strings.Compare(strings.ToLower(left.Sentiment), strings.ToLower(right.Sentiment))
	}
	var result int
	switch sortKey {
	case "observed_at":
		result = compareTime(left.ObservedAt, right.ObservedAt)
	case "published_at":
		result = compareTime(left.PublishedAt, right.PublishedAt)
	case "likes":
		result = compareOptionalInt(left.Likes, right.Likes)
	case "comments":
		result = compareOptionalInt(left.Comments, right.Comments)
	case "favorites":
		result = compareOptionalInt(left.Favorites, right.Favorites)
	case "forwards":
		result = compareOptionalInt(left.Forwards, right.Forwards)
	case "growth_rate":
		result = compareOptionalFloat(left.GrowthRate, right.GrowthRate)
	case "interactions":
		result = compareOptionalInt(left.Performance.Interactions, right.Performance.Interactions)
	case "growth_per_hour":
		result = compareOptionalFloat(left.Performance.GrowthPerHour, right.Performance.GrowthPerHour)
	case "relative_performance":
		result = compareOptionalFloat(left.Performance.RelativeToMedian, right.Performance.RelativeToMedian)
	case "recent_heat":
		result = compareOptionalFloat(left.Performance.RecentHeat.Score, right.Performance.RecentHeat.Score)
	case "related_videos":
		result = compareOptionalInt(left.RelatedVideos, right.RelatedVideos)
	case "video_count":
		result = compareOptionalInt(left.VideoCount, right.VideoCount)
	case "question_rate":
		result = compareOptionalInt(left.QuestionRate, right.QuestionRate)
	}
	if result == 0 {
		return strings.Compare(left.ID, right.ID)
	}
	return result
}

func compareTime(left, right time.Time) int {
	switch {
	case left.Before(right):
		return -1
	case left.After(right):
		return 1
	default:
		return 0
	}
}

func compareOptionalInt(left, right *int) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}

func compareOptionalFloat(left, right *float64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}

func paginateDataCenter(total, page, pageSize int) (int, int, int) {
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return page, pageSize, totalPages
}

type dataCenterBuildResult struct {
	rows          map[DataCenterEntity]map[string]DataCenterItem
	histories     map[string][]DataCenterHistoryItem
	accounts      map[string]*dataCenterAccountState
	topicVideos   map[string]map[string]struct{}
	topicAccounts map[string]map[string]string
}

type dataCenterAccountState struct {
	item       DataCenterItem
	videos     map[string]DataCenterItem
	sources    map[string]struct{}
	historyIDs map[string]struct{}
}

func (s *DataCenterService) buildRows(ctx context.Context, workspaceID, hubUserID string) (map[DataCenterEntity][]DataCenterItem, map[string][]DataCenterHistoryItem, error) {
	return s.buildRowsWithRawComments(ctx, workspaceID, hubUserID, true, false)
}

func (s *DataCenterService) buildRowsWithoutRawComments(ctx context.Context, workspaceID, hubUserID string) (map[DataCenterEntity][]DataCenterItem, map[string][]DataCenterHistoryItem, error) {
	return s.buildRowsWithRawComments(ctx, workspaceID, hubUserID, false, true)
}

func (s *DataCenterService) buildRowsWithRawComments(ctx context.Context, workspaceID, hubUserID string, includeRawComments, latestObservations bool) (map[DataCenterEntity][]DataCenterItem, map[string][]DataCenterHistoryItem, error) {
	if s == nil || s.dataProvider == nil {
		return nil, nil, ErrDataCenterServiceUnavailable
	}
	events, err := s.dataProvider.ListEvents(ctx, ingest.SummaryQuery{HubUserID: hubUserID})
	if err != nil {
		return nil, nil, err
	}
	var observations []ingest.VideoMetricObservation
	if latestObservations {
		if provider, ok := s.dataProvider.(DataCenterLatestVideoMetricProvider); ok {
			observations, err = provider.ListLatestVideoMetricObservations(ctx, ingest.SummaryQuery{HubUserID: hubUserID})
		} else {
			observations, err = s.dataProvider.ListVideoMetricObservations(ctx, ingest.SummaryQuery{HubUserID: hubUserID})
		}
	} else {
		observations, err = s.dataProvider.ListVideoMetricObservations(ctx, ingest.SummaryQuery{HubUserID: hubUserID})
	}
	if err != nil {
		return nil, nil, err
	}
	result := dataCenterBuildResult{
		rows: map[DataCenterEntity]map[string]DataCenterItem{
			DataCenterEntityVideos: {}, DataCenterEntityTopics: {}, DataCenterEntityAccounts: {}, DataCenterEntityComments: {},
		},
		histories:     map[string][]DataCenterHistoryItem{},
		accounts:      map[string]*dataCenterAccountState{},
		topicVideos:   map[string]map[string]struct{}{},
		topicAccounts: map[string]map[string]string{},
	}
	for _, event := range events {
		switch event.EventType {
		case "video_snapshot":
			result.addVideo(eventItemFromIngestEvent(event), historyFromItem(eventItemFromIngestEvent(event), "event", event.ID))
		case "topic_cluster":
			result.addTopicEvent(event)
		case "comment_snapshot":
			result.addCommentEvent(event)
		}
		result.addAccountEvent(event)
	}
	for _, observation := range observations {
		item := itemFromVideoObservation(observation)
		result.addVideo(item, historyFromItem(item, "metric_observation", observation.ID))
	}
	if includeRawComments && s.rawCommentProvider != nil {
		if err := s.addRawComments(ctx, hubUserID, &result); err != nil {
			return nil, nil, err
		}
	}
	result.finalizeTopics()
	result.finalizeAccounts()
	if s.accountProvider != nil {
		if err := s.mergeAccountMasters(ctx, workspaceID, &result); err != nil {
			return nil, nil, err
		}
	}
	rows := map[DataCenterEntity][]DataCenterItem{}
	for entity, values := range result.rows {
		items := make([]DataCenterItem, 0, len(values))
		for id, item := range values {
			item.HistoryAvailable = len(result.histories[id]) > 0
			items = append(items, item)
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		rows[entity] = items
	}
	for id, items := range result.histories {
		sort.SliceStable(items, func(i, j int) bool { return items[i].ObservedAt.After(items[j].ObservedAt) })
		result.histories[id] = dedupeDataCenterHistory(items)
	}
	return rows, result.histories, nil
}

func (s *DataCenterService) addRawComments(ctx context.Context, hubUserID string, result *dataCenterBuildResult) error {
	if result == nil || s.rawCommentProvider == nil {
		return nil
	}
	// Raw comments are the canonical data-center comments when the repository
	// is available. Keep the older insight-event comments only for providers
	// that do not expose raw records (demo and legacy tests).
	result.rows[DataCenterEntityComments] = map[string]DataCenterItem{}
	for page := 1; ; page++ {
		payload, err := s.rawCommentProvider.List(ctx, hubUserID, ingest.RawCommentQuery{Page: page, PageSize: 100})
		if err != nil {
			return err
		}
		for _, record := range payload.Items {
			item := itemFromRawComment(record)
			if item.ID == "" {
				continue
			}
			result.rows[DataCenterEntityComments][item.ID] = item
			result.histories[item.ID] = append(result.histories[item.ID], historyFromItem(item, "raw_comment", record.ID))
			result.addAccountForItem(item)
		}
		if payload.Page >= payload.TotalPages || len(payload.Items) == 0 {
			break
		}
	}
	return nil
}

func (s *DataCenterService) mergeAccountMasters(ctx context.Context, workspaceID string, result *dataCenterBuildResult) error {
	if result == nil || s.accountProvider == nil {
		return nil
	}
	accounts, err := s.accountProvider.List(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		legacyID := dataCenterAccountID(accountKey(account.ChannelUsername, account.ChannelName))
		item, found := result.rows[DataCenterEntityAccounts][legacyID]
		if found {
			delete(result.rows[DataCenterEntityAccounts], legacyID)
			if history, ok := result.histories[legacyID]; ok {
				result.histories[account.ID] = append(result.histories[account.ID], history...)
				delete(result.histories, legacyID)
			}
			item.ID = account.ID
		} else {
			item = DataCenterItem{ID: account.ID, Entity: DataCenterEntityAccounts}
		}
		item.AccountID = account.ID
		item.IdentityStatus = string(account.IdentityStatus)
		item.ChannelUsername = account.ChannelUsername
		item.ChannelName = account.ChannelName
		item.Region = account.Region
		item.Industry = account.Industry
		item.Tags = uniqueStrings(append(item.Tags, account.Tags...))
		item.Note = account.Note
		item.IsPriority = account.IsPriority
		item.ProfileSource = string(account.ProfileSource)
		if !found {
			item.ObservedAt = account.ProfileObservedAt
		}
		if item.Source == "" {
			item.Source = "account_master"
		}
		item.Sources = uniqueStrings(append(item.Sources, "account_master"))
		if item.DataBasis == "" {
			item.DataBasis = "账户主表画像；指标仅在有视频观察证据时显示。"
		} else {
			item.DataBasis += " 账户画像来自 insight_accounts。"
		}
		result.rows[DataCenterEntityAccounts][account.ID] = item
	}
	return nil
}

func (r *dataCenterBuildResult) addVideo(item DataCenterItem, history DataCenterHistoryItem) {
	if item.ID == "" {
		return
	}
	r.rows[DataCenterEntityVideos][item.ID] = mergeDataCenterItems(r.rows[DataCenterEntityVideos][item.ID], item)
	r.histories[item.ID] = append(r.histories[item.ID], history)
	r.addAccountForItem(item)
	for _, keyword := range uniqueStrings(append([]string{item.Keyword}, append(item.Tags, item.Keywords...)...)) {
		r.addTopicCandidate(keyword, item, history)
	}
}

func (r *dataCenterBuildResult) addTopicEvent(event ingest.Event) {
	item := itemFromTopicEvent(event)
	if item.Keyword != "" {
		r.addTopicCandidate(item.Keyword, item, historyFromItem(item, "event", event.ID))
	}
	for _, keyword := range uniqueStrings(append(append([]string(nil), event.Keywords...), event.Tags...)) {
		r.addTopicCandidate(keyword, item, historyFromItem(item, "event", event.ID))
	}
	r.addAccountEvent(event)
}

func (r *dataCenterBuildResult) addTopicCandidate(keyword string, source DataCenterItem, history DataCenterHistoryItem) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return
	}
	id := dataCenterTopicID(keyword)
	item := DataCenterItem{
		ID:            id,
		Entity:        DataCenterEntityTopics,
		Source:        source.Source,
		Sources:       []string{source.Source},
		ObservedAt:    source.ObservedAt,
		Keyword:       keyword,
		Cluster:       source.Cluster,
		Comments:      cloneOptionalInt(source.Comments),
		Mentions:      intPtr(maxDataCenterInt(1, valueOrZero(source.Comments))),
		RelatedVideos: intPtr(1),
		GrowthRate:    cloneOptionalFloat(source.GrowthRate),
		DataBasis:     "来自真实事件或视频标签的全量话题观察。",
	}
	for _, account := range source.SourceAccounts {
		r.addTopicAccount(id, account)
	}
	r.addTopicAccount(id, source.ChannelName)
	if source.Entity == DataCenterEntityVideos {
		item.RelatedVideos = intPtr(1)
	}
	if source.Title != "" {
		item.Title = source.Title
	}
	r.rows[DataCenterEntityTopics][id] = mergeDataCenterItems(r.rows[DataCenterEntityTopics][id], item)
	if r.topicVideos[id] == nil {
		r.topicVideos[id] = map[string]struct{}{}
	}
	if source.Entity == DataCenterEntityVideos {
		r.topicVideos[id][source.ID] = struct{}{}
	}
	r.histories[id] = append(r.histories[id], DataCenterHistoryItem{
		ID:            history.ID,
		EntityID:      id,
		ObservedAt:    history.ObservedAt,
		Source:        history.Source,
		EvidenceType:  history.EvidenceType,
		Keyword:       keyword,
		Title:         history.Title,
		ChannelName:   history.ChannelName,
		Comments:      cloneOptionalInt(history.Comments),
		RelatedVideos: cloneOptionalInt(history.RelatedVideos),
	})
	if source.Entity == DataCenterEntityVideos {
		updatedTopic := r.rows[DataCenterEntityTopics][id]
		updatedTopic.RelatedVideos = intPtr(len(r.topicVideos[id]))
		r.rows[DataCenterEntityTopics][id] = updatedTopic
	}
}

func (r *dataCenterBuildResult) addTopicAccount(topicID, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if r.topicAccounts[topicID] == nil {
		r.topicAccounts[topicID] = map[string]string{}
	}
	key := strings.ToLower(value)
	if _, exists := r.topicAccounts[topicID][key]; !exists {
		r.topicAccounts[topicID][key] = value
	}
}

func (r *dataCenterBuildResult) finalizeTopics() {
	for id, item := range r.rows[DataCenterEntityTopics] {
		accounts := r.topicAccounts[id]
		item.SourceAccounts = make([]string, 0, len(accounts))
		for _, account := range accounts {
			item.SourceAccounts = append(item.SourceAccounts, account)
		}
		sort.Strings(item.SourceAccounts)
		r.rows[DataCenterEntityTopics][id] = item
	}
}

func (r *dataCenterBuildResult) addCommentEvent(event ingest.Event) {
	item := itemFromCommentEvent(event)
	if item.ID == "" {
		return
	}
	r.rows[DataCenterEntityComments][item.ID] = mergeDataCenterItems(r.rows[DataCenterEntityComments][item.ID], item)
	r.histories[item.ID] = append(r.histories[item.ID], historyFromItem(item, "event", event.ID))
}

func (r *dataCenterBuildResult) addAccountEvent(event ingest.Event) {
	item := eventItemFromIngestEvent(event)
	if event.EventType == "comment_snapshot" {
		item = itemFromCommentEvent(event)
	}
	if event.EventType == "topic_cluster" {
		item = itemFromTopicEvent(event)
	}
	r.addAccountForItem(item)
}

func (r *dataCenterBuildResult) addAccountForItem(source DataCenterItem) {
	key := accountKey(source.ChannelUsername, source.ChannelName)
	if key == "" {
		return
	}
	id := dataCenterAccountID(key)
	state := r.accounts[id]
	if state == nil {
		state = &dataCenterAccountState{
			item:   DataCenterItem{ID: id, Entity: DataCenterEntityAccounts, DataBasis: "由全量视频、话题和评论事件聚合；包含已监控和未监控账号。"},
			videos: map[string]DataCenterItem{}, sources: map[string]struct{}{}, historyIDs: map[string]struct{}{},
		}
		r.accounts[id] = state
	}
	state.item = mergeDataCenterItems(state.item, DataCenterItem{
		ID: id, Entity: DataCenterEntityAccounts, Source: source.Source, ObservedAt: source.ObservedAt,
		ChannelName: source.ChannelName, ChannelUsername: source.ChannelUsername,
	})
	if source.Source != "" {
		state.sources[source.Source] = struct{}{}
	}
	if source.Entity == DataCenterEntityVideos && source.ID != "" {
		state.videos[source.ID] = mergeDataCenterItems(state.videos[source.ID], source)
	}
	historyID := source.ID + "|" + source.Source + "|" + source.ObservedAt.UTC().Format(time.RFC3339Nano)
	if _, ok := state.historyIDs[historyID]; ok {
		return
	}
	state.historyIDs[historyID] = struct{}{}
	r.histories[id] = append(r.histories[id], DataCenterHistoryItem{
		ID: source.ID, EntityID: id, ObservedAt: source.ObservedAt, Source: source.Source,
		EvidenceType: "event", Title: source.Title, ChannelName: source.ChannelName, ChannelUsername: source.ChannelUsername,
		Views: cloneOptionalInt(source.Views), Likes: cloneOptionalInt(source.Likes), Comments: cloneOptionalInt(source.Comments),
		Favorites: cloneOptionalInt(source.Favorites), Forwards: cloneOptionalInt(source.Forwards), VideoCount: intPtr(1),
	})
}

func (r *dataCenterBuildResult) finalizeAccounts() {
	for id, state := range r.accounts {
		item := state.item
		videoCount := len(state.videos)
		likes, comments, favorites, forwards := 0, 0, 0, 0
		var missing ingest.MetricMask
		for _, video := range state.videos {
			if video.Likes == nil {
				missing |= ingest.MetricLike
			}
			if video.Comments == nil {
				missing |= ingest.MetricComment
			}
			if video.Favorites == nil {
				missing |= ingest.MetricFavorite
			}
			if video.Forwards == nil {
				missing |= ingest.MetricForward
			}
			likes += valueOrZero(video.Likes)
			comments += valueOrZero(video.Comments)
			favorites += valueOrZero(video.Favorites)
			forwards += valueOrZero(video.Forwards)
		}
		item.VideoCount = intPtr(videoCount)
		item.Likes, item.Comments, item.Favorites, item.Forwards = ingest.MetricValue(likes, missing, ingest.MetricLike), ingest.MetricValue(comments, missing, ingest.MetricComment), ingest.MetricValue(favorites, missing, ingest.MetricFavorite), ingest.MetricValue(forwards, missing, ingest.MetricForward)
		item.Sources = sortedMapKeys(state.sources)
		item.HistoryAvailable = len(r.histories[id]) > 0
		r.rows[DataCenterEntityAccounts][id] = item
	}
}

func eventItemFromIngestEvent(event ingest.Event) DataCenterItem {
	return DataCenterItem{
		ID: dataCenterVideoID(event.ObjectID, event.ObjectNonceID, event.ID), Entity: DataCenterEntityVideos,
		Source: event.Source, Sources: exactStrings([]string{event.Source}), SourceKeywords: exactStrings([]string{event.Keyword}), ObservedAt: dataCenterEventTime(event), PublishedAt: event.PublishedAt,
		ObjectID: event.ObjectID, ObjectNonceID: event.ObjectNonceID, CoverURL: event.CoverURL,
		Title: event.Title, ChannelName: event.ChannelName, ChannelUsername: event.ChannelUsername,
		Keyword: event.Keyword, Signal: event.Signal, Tags: append([]string(nil), event.Tags...), Keywords: append([]string(nil), event.Keywords...),
		Views: dataCenterViews(event.Source, event.PlayCount), Likes: ingest.MetricValue(event.LikeCount, event.MissingMetrics, ingest.MetricLike),
		Comments: ingest.MetricValue(event.CommentCount, event.MissingMetrics, ingest.MetricComment), Favorites: ingest.MetricValue(event.FavCount, event.MissingMetrics, ingest.MetricFavorite),
		Forwards:   ingest.MetricValue(event.ForwardCount, event.MissingMetrics, ingest.MetricForward),
		VideoCount: intPtr(maxDataCenterInt(1, event.VideoCount)), DataBasis: "来自真实 ingest 事件。",
	}
}

func itemFromVideoObservation(observation ingest.VideoMetricObservation) DataCenterItem {
	return DataCenterItem{
		ID: dataCenterVideoID(observation.ObjectID, observation.ObjectNonceID, observation.ID), Entity: DataCenterEntityVideos,
		Source: observation.Source, Sources: exactStrings([]string{observation.Source}), SourceKeywords: exactStrings([]string{observation.Keyword}), ObservedAt: observation.ObservedAt, PublishedAt: observation.PublishedAt,
		ObjectID: observation.ObjectID, ObjectNonceID: observation.ObjectNonceID, CoverURL: observation.CoverURL,
		Title: observation.Title, ChannelName: observation.ChannelName, ChannelUsername: observation.ChannelUsername,
		Keyword: observation.Keyword, Tags: append([]string(nil), observation.Tags...),
		Views: dataCenterViews(observation.Source, observation.PlayCount), Likes: ingest.MetricValue(observation.LikeCount, observation.MissingMetrics, ingest.MetricLike),
		Comments: ingest.MetricValue(observation.CommentCount, observation.MissingMetrics, ingest.MetricComment), Favorites: ingest.MetricValue(observation.FavCount, observation.MissingMetrics, ingest.MetricFavorite),
		Forwards: ingest.MetricValue(observation.ForwardCount, observation.MissingMetrics, ingest.MetricForward), VideoCount: intPtr(1),
		DataBasis: "来自追加式 VideoMetricObservation。",
	}
}

// Legacy scalar counters cannot distinguish absent WX views from zero. Keep
// unknown zero values absent; a positive upstream value is actual evidence.
func dataCenterViews(source string, count int) *int {
	if isRecommendationVideoSource(source) && count <= 0 {
		return nil
	}
	return intPtr(maxDataCenterInt(0, count))
}

func videoFactsInWindow(item DataCenterItem, history []DataCenterHistoryItem, from, to time.Time) (DataCenterItem, bool) {
	metricHistory := false
	for _, point := range history {
		if point.EvidenceType == "metric_observation" {
			metricHistory = true
			break
		}
	}
	observations := make([]ingest.VideoMetricObservation, 0, len(history))
	points := map[string]DataCenterHistoryItem{}
	for _, point := range history {
		if metricHistory && point.EvidenceType != "metric_observation" {
			continue
		}
		// Evidence IDs are immutable, unlike the mutable latest event identity.
		key := point.ID + "|" + point.ObservedAt.UTC().Format(time.RFC3339Nano)
		points[key] = point
		var missing ingest.MetricMask
		if point.Likes == nil {
			missing |= ingest.MetricLike
		}
		if point.Comments == nil {
			missing |= ingest.MetricComment
		}
		if point.Favorites == nil {
			missing |= ingest.MetricFavorite
		}
		if point.Forwards == nil {
			missing |= ingest.MetricForward
		}
		observations = append(observations, ingest.VideoMetricObservation{
			MissingMetrics: missing,
			ID:             key, ObjectID: item.ObjectID, Source: point.Source, ObservedAt: point.ObservedAt,
			LikeCount: valueOrZero(point.Likes), CommentCount: valueOrZero(point.Comments),
			FavCount: valueOrZero(point.Favorites), ForwardCount: valueOrZero(point.Forwards),
		})
	}
	growth := qualifyVideoGrowth(observations, from, to)
	item.growthInteractions = nil
	if hasComparableInteractionCounts(growth.Status) {
		delta := growth.Deltas.Likes + growth.Deltas.Comments + growth.Deltas.Favorites + growth.Deltas.Forwards
		item.growthInteractions = &delta
	}
	item.GrowthRate, item.GrowthStatus, item.GrowthBasis, item.GrowthWindow = growth.Rate, growth.Status, growth.Basis, growth.Window
	item.ObservationCount = growth.ObservationCount
	if growth.Latest == nil {
		return item, from.IsZero() && to.IsZero()
	}
	latest := points[growth.Latest.ID]
	item.ObservedAt, item.Source = latest.ObservedAt, latest.Source
	item.Title, item.ChannelName, item.ChannelUsername = latest.Title, latest.ChannelName, latest.ChannelUsername
	item.Views, item.Likes, item.Comments = cloneOptionalInt(latest.Views), cloneOptionalInt(latest.Likes), cloneOptionalInt(latest.Comments)
	item.Favorites, item.Forwards = cloneOptionalInt(latest.Favorites), cloneOptionalInt(latest.Forwards)
	if metricHistory {
		item.DataBasis = "来自追加式 VideoMetricObservation。"
	}
	return item, true
}

func containsExact(values []string, needle string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == needle {
			return true
		}
	}
	return false
}

func exactStrings(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = struct{}{}
		}
	}
	return sortedMapKeys(set)
}

func itemFromTopicEvent(event ingest.Event) DataCenterItem {
	return DataCenterItem{
		ID: dataCenterTopicID(event.Keyword), Entity: DataCenterEntityTopics, Source: event.Source,
		Sources:    []string{event.Source},
		ObservedAt: dataCenterEventTime(event), Keyword: strings.TrimSpace(event.Keyword), Cluster: event.Cluster,
		Signal: event.Signal, Comments: ingest.MetricValue(event.CommentCount, event.MissingMetrics, ingest.MetricComment),
		Mentions: intPtr(maxDataCenterInt(1, event.TopicCount)), RelatedVideos: intPtr(maxDataCenterInt(0, event.VideoCount)),
		SourceAccounts: dataCenterSourceAccountsFromEvent(event), Keywords: append([]string(nil), event.Keywords...),
		DataBasis: "来自真实 topic_cluster 事件。",
	}
}

func itemFromCommentEvent(event ingest.Event) DataCenterItem {
	theme := strings.TrimSpace(event.Keyword)
	if theme == "" {
		theme = strings.TrimSpace(event.Title)
	}
	if theme == "" {
		theme = "评论洞察"
	}
	questionRate := minDataCenterInt(88, maxDataCenterInt(35, 20+maxDataCenterInt(0, event.CommentCount)/4))
	return DataCenterItem{
		ID: dataCenterCommentID(event), Entity: DataCenterEntityComments, Source: event.Source,
		ObservedAt: dataCenterEventTime(event), Title: event.Title, ChannelName: event.ChannelName,
		ChannelUsername: event.ChannelUsername, Keyword: event.Keyword, Theme: theme,
		Representative: event.RepresentativeComment, Sentiment: event.Signal, Signal: event.Signal,
		SourceAccounts: dataCenterSourceAccountsFromEvent(event), RelatedKeywords: append([]string(nil), event.Keywords...),
		Comments: ingest.MetricValue(event.CommentCount, event.MissingMetrics, ingest.MetricComment), QuestionRate: intPtr(questionRate),
		DataBasis: "结构化评论洞察记录；不是逐条原始评论。",
	}
}

func itemFromRawComment(record ingest.RawCommentRecord) DataCenterItem {
	commentID := strings.TrimSpace(record.CommentID)
	objectID := strings.TrimSpace(record.VideoObjectID)
	if commentID == "" || objectID == "" {
		return DataCenterItem{}
	}
	observedAt := record.CollectedAt
	if observedAt.IsZero() {
		observedAt = record.CreatedAt
	}
	return DataCenterItem{
		ID:               "comment:" + objectID + "\x00" + commentID,
		Entity:           DataCenterEntityComments,
		Source:           record.Source,
		Sources:          []string{record.Source},
		ObservedAt:       observedAt,
		PublishedAt:      record.CreatedAt,
		ObjectID:         objectID,
		ObjectNonceID:    record.VideoObjectNonceID,
		Title:            record.VideoTitle,
		ChannelName:      record.ChannelName,
		ChannelUsername:  record.ChannelUsername,
		AccountID:        record.AccountID,
		CommentID:        commentID,
		ParentCommentID:  record.ParentCommentID,
		AuthorUsername:   record.AuthorUsername,
		AuthorNickname:   record.AuthorNickname,
		Content:          record.Content,
		IPRegion:         record.IPRegion,
		CommentCreatedAt: record.CreatedAt,
		CommentLikeCount: intPtr(record.LikeCount),
		ReplyCount:       intPtr(record.ReplyCount),
		Likes:            intPtr(record.LikeCount),
		Comments:         intPtr(record.ReplyCount),
		DataBasis:        "逐条原始评论事实；与评论洞察分析分层保存。",
	}
}

func historyFromItem(item DataCenterItem, evidenceType, evidenceID string) DataCenterHistoryItem {
	return DataCenterHistoryItem{
		ID: evidenceID, EntityID: item.ID, ObservedAt: item.ObservedAt, Source: item.Source, EvidenceType: evidenceType,
		Title: item.Title, ChannelName: item.ChannelName, ChannelUsername: item.ChannelUsername,
		Keyword: item.Keyword, Theme: item.Theme, Representative: item.Representative, Signal: item.Signal,
		Views: cloneOptionalInt(item.Views), Likes: cloneOptionalInt(item.Likes), Comments: cloneOptionalInt(item.Comments),
		Favorites: cloneOptionalInt(item.Favorites), Forwards: cloneOptionalInt(item.Forwards), VideoCount: cloneOptionalInt(item.VideoCount),
		Mentions: cloneOptionalInt(item.Mentions), RelatedVideos: cloneOptionalInt(item.RelatedVideos), QuestionRate: cloneOptionalInt(item.QuestionRate),
	}
}

func mergeDataCenterItems(existing, candidate DataCenterItem) DataCenterItem {
	if existing.ID == "" {
		candidate.SourceAccounts = uniqueStrings(candidate.SourceAccounts)
		candidate.Tags = uniqueStrings(candidate.Tags)
		candidate.Keywords = uniqueStrings(candidate.Keywords)
		candidate.Sources = uniqueStrings(candidate.Sources)
		candidate.SourceKeywords = exactStrings(candidate.SourceKeywords)
		return candidate
	}
	newer := candidate
	older := existing
	if candidate.ObservedAt.Before(existing.ObservedAt) {
		newer, older = existing, candidate
	}
	merged := newer
	if merged.PublishedAt.IsZero() {
		merged.PublishedAt = older.PublishedAt
	}
	if merged.Source == "" {
		merged.Source = older.Source
	}
	if merged.Title == "" {
		merged.Title = older.Title
	}
	if merged.ChannelName == "" {
		merged.ChannelName = older.ChannelName
	}
	if merged.ChannelUsername == "" {
		merged.ChannelUsername = older.ChannelUsername
	}
	if merged.ObjectID == "" {
		merged.ObjectID = older.ObjectID
	}
	if merged.ObjectNonceID == "" {
		merged.ObjectNonceID = older.ObjectNonceID
	}
	if merged.CoverURL == "" {
		merged.CoverURL = older.CoverURL
	}
	if merged.Keyword == "" {
		merged.Keyword = older.Keyword
	}
	if merged.Cluster == "" {
		merged.Cluster = older.Cluster
	}
	if merged.Theme == "" {
		merged.Theme = older.Theme
	}
	if merged.Representative == "" {
		merged.Representative = older.Representative
	}
	if merged.Sentiment == "" {
		merged.Sentiment = older.Sentiment
	}
	if merged.Signal == "" {
		merged.Signal = older.Signal
	}
	if merged.DataBasis == "" {
		merged.DataBasis = older.DataBasis
	}
	merged.Sources = uniqueStrings(append(append([]string(nil), newer.Sources...), older.Sources...))
	merged.SourceKeywords = exactStrings(append(append([]string(nil), newer.SourceKeywords...), older.SourceKeywords...))
	merged.SourceAccounts = uniqueStrings(append(append([]string(nil), newer.SourceAccounts...), older.SourceAccounts...))
	merged.Tags = uniqueStrings(append(append([]string(nil), newer.Tags...), older.Tags...))
	merged.Keywords = uniqueStrings(append(append([]string(nil), newer.Keywords...), older.Keywords...))
	merged.RelatedKeywords = uniqueStrings(append(append([]string(nil), newer.RelatedKeywords...), older.RelatedKeywords...))
	if merged.Entity != DataCenterEntityVideos && merged.Views == nil {
		merged.Views = cloneOptionalInt(older.Views)
	}
	if merged.Entity != DataCenterEntityVideos && merged.Likes == nil {
		merged.Likes = cloneOptionalInt(older.Likes)
	}
	if merged.Entity != DataCenterEntityVideos && merged.Comments == nil {
		merged.Comments = cloneOptionalInt(older.Comments)
	}
	if merged.Entity != DataCenterEntityVideos && merged.Favorites == nil {
		merged.Favorites = cloneOptionalInt(older.Favorites)
	}
	if merged.Entity != DataCenterEntityVideos && merged.Forwards == nil {
		merged.Forwards = cloneOptionalInt(older.Forwards)
	}
	if merged.VideoCount == nil {
		merged.VideoCount = cloneOptionalInt(older.VideoCount)
	}
	if merged.Mentions == nil {
		merged.Mentions = cloneOptionalInt(older.Mentions)
	}
	if merged.RelatedVideos == nil {
		merged.RelatedVideos = cloneOptionalInt(older.RelatedVideos)
	}
	if merged.QuestionRate == nil {
		merged.QuestionRate = cloneOptionalInt(older.QuestionRate)
	}
	if merged.GrowthRate == nil {
		merged.GrowthRate = cloneOptionalFloat(older.GrowthRate)
	}
	return merged
}

func dedupeDataCenterHistory(items []DataCenterHistoryItem) []DataCenterHistoryItem {
	seen := map[string]struct{}{}
	result := make([]DataCenterHistoryItem, 0, len(items))
	for _, item := range items {
		key := item.EvidenceType + "|" + item.ID + "|" + item.ObservedAt.UTC().Format(time.RFC3339Nano)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func dataCenterEventTime(event ingest.Event) time.Time {
	if !event.CollectedAt.IsZero() {
		return event.CollectedAt
	}
	return event.PublishedAt
}

func dataCenterVideoID(objectID, nonceID, fallback string) string {
	for _, value := range []string{objectID, nonceID, fallback} {
		if value = strings.TrimSpace(value); value != "" {
			return "video:" + value
		}
	}
	return ""
}

func dataCenterTopicID(keyword string) string {
	return "topic:" + strings.ToLower(strings.TrimSpace(keyword))
}

func dataCenterAccountID(key string) string {
	return "account:" + strings.ToLower(strings.TrimSpace(key))
}

func dataCenterCommentID(event ingest.Event) string {
	base := strings.TrimSpace(event.ID)
	if base == "" {
		base = strings.TrimSpace(event.BatchID)
	}
	if base == "" {
		base = strings.TrimSpace(event.ObjectID) + "-" + strings.TrimSpace(event.Keyword)
	}
	return "comment:" + base
}

func accountKey(username, name string) string {
	for _, value := range []string{username, name} {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			return value
		}
	}
	return ""
}

func dataCenterSourceAccountsFromEvent(event ingest.Event) []string {
	values := []string{}
	for _, value := range []string{event.ChannelUsername, event.ChannelName} {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return uniqueStrings(values)
}

func containsFold(values []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	for _, value := range values {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), needle) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedMapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneDataCenterItems(items []DataCenterItem) []DataCenterItem {
	result := make([]DataCenterItem, len(items))
	for index, item := range items {
		result[index] = item
		result[index].Sources = append([]string(nil), item.Sources...)
		result[index].SourceKeywords = append([]string(nil), item.SourceKeywords...)
		result[index].SourceAccounts = append([]string(nil), item.SourceAccounts...)
		result[index].RelatedKeywords = append([]string(nil), item.RelatedKeywords...)
		result[index].Tags = append([]string(nil), item.Tags...)
		result[index].Keywords = append([]string(nil), item.Keywords...)
		result[index].Views = cloneOptionalInt(item.Views)
		result[index].Likes = cloneOptionalInt(item.Likes)
		result[index].Comments = cloneOptionalInt(item.Comments)
		result[index].Favorites = cloneOptionalInt(item.Favorites)
		result[index].Forwards = cloneOptionalInt(item.Forwards)
		result[index].VideoCount = cloneOptionalInt(item.VideoCount)
		result[index].Mentions = cloneOptionalInt(item.Mentions)
		result[index].RelatedVideos = cloneOptionalInt(item.RelatedVideos)
		result[index].QuestionRate = cloneOptionalInt(item.QuestionRate)
		result[index].GrowthRate = cloneOptionalFloat(item.GrowthRate)
	}
	return result
}

func cloneOptionalInt(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func cloneOptionalFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func intPtr(value int) *int { return &value }
func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
func maxDataCenterInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
func minDataCenterInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// FilterDashboardByDataCenterExclusions keeps analysis consumers aligned with
// the data-center live-state mutation without rewriting immutable facts.
func FilterDashboardByDataCenterExclusions(ctx context.Context, payload *Dashboard, workspaceID string, repo DataCenterExclusionRepository) error {
	if payload == nil || repo == nil {
		return nil
	}
	items, err := repo.List(ctx, workspaceID)
	if err != nil {
		return err
	}
	set := map[string]struct{}{}
	for _, item := range items {
		set[exclusionKey(item.Entity, item.EntityID)] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	payload.HotVideos = filterHotVideosByExclusions(payload.HotVideos, set)
	for key, videos := range payload.HotVideoRankings {
		payload.HotVideoRankings[key] = filterHotVideosByExclusions(videos, set)
	}
	payload.HotTopics = filterHotTopicsByExclusions(payload.HotTopics, set)
	payload.AccountRecommendations = filterAccountRecommendationsByExclusions(payload.AccountRecommendations, set)
	payload.VideoRecommendations = filterVideoRecommendationsByExclusions(payload.VideoRecommendations, set)
	payload.CommentInsights = filterCommentInsightsByExclusions(payload.CommentInsights, set)
	return nil
}

func isDataCenterExcluded(set map[string]struct{}, entity DataCenterEntity, id string) bool {
	_, ok := set[exclusionKey(entity, id)]
	return ok
}

func filterHotVideosByExclusions(items []HotVideo, set map[string]struct{}) []HotVideo {
	result := make([]HotVideo, 0, len(items))
	for _, item := range items {
		if isDataCenterExcluded(set, DataCenterEntityVideos, item.ID) || isDataCenterExcluded(set, DataCenterEntityVideos, dataCenterVideoID(item.ObjectID, item.ObjectNonceID, item.ID)) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func filterHotTopicsByExclusions(items []HotTopic, set map[string]struct{}) []HotTopic {
	result := make([]HotTopic, 0, len(items))
	for _, item := range items {
		if isDataCenterExcluded(set, DataCenterEntityTopics, item.ID) || isDataCenterExcluded(set, DataCenterEntityTopics, dataCenterTopicID(item.Keyword)) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func filterAccountRecommendationsByExclusions(items []AccountRecommendation, set map[string]struct{}) []AccountRecommendation {
	result := make([]AccountRecommendation, 0, len(items))
	for _, item := range items {
		if isDataCenterExcluded(set, DataCenterEntityAccounts, item.ID) || isDataCenterExcluded(set, DataCenterEntityAccounts, dataCenterAccountID(accountKey(item.Username, item.ChannelName))) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func filterVideoRecommendationsByExclusions(items []VideoRecommendation, set map[string]struct{}) []VideoRecommendation {
	result := make([]VideoRecommendation, 0, len(items))
	for _, item := range items {
		if isDataCenterExcluded(set, DataCenterEntityVideos, item.ID) || isDataCenterExcluded(set, DataCenterEntityVideos, dataCenterVideoID(item.ObjectID, item.ObjectNonceID, item.ID)) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func filterCommentInsightsByExclusions(items []CommentInsight, set map[string]struct{}) []CommentInsight {
	result := make([]CommentInsight, 0, len(items))
	for _, item := range items {
		if isDataCenterExcluded(set, DataCenterEntityComments, item.ID) || isDataCenterExcluded(set, DataCenterEntityComments, "comment:"+item.ID) {
			continue
		}
		if isDataCenterExcluded(set, DataCenterEntityComments, "comment:"+strings.TrimSpace(item.Theme)) ||
			isDataCenterExcluded(set, DataCenterEntityComments, "comment:"+strings.TrimSpace(item.ObjectID)) {
			continue
		}
		if isDataCenterExcluded(set, DataCenterEntityComments, dataCenterCommentID(ingest.Event{ID: item.ID, Keyword: item.Theme, ObjectID: item.ObjectID})) {
			continue
		}
		result = append(result, item)
	}
	return result
}
