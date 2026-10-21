package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

const accountRoutePrefix = "/api/v1/accounts"

type accountDerivedMetrics struct {
	VideoCount       *int      `json:"videoCount,omitempty"`
	Likes            *int      `json:"likes,omitempty"`
	Comments         *int      `json:"comments,omitempty"`
	Favorites        *int      `json:"favorites,omitempty"`
	Forwards         *int      `json:"forwards,omitempty"`
	RecentObservedAt time.Time `json:"recentObservedAt,omitempty"`
	DataBasis        string    `json:"dataBasis,omitempty"`
}

type accountAPIItem struct {
	insight.Account
	Metrics    *accountDerivedMetrics `json:"metrics,omitempty"`
	Monitoring map[string]any         `json:"monitoring,omitempty"`
}

type accountAPIPage struct {
	Items      []accountAPIItem `json:"items"`
	Page       int              `json:"page"`
	PageSize   int              `json:"pageSize"`
	Total      int              `json:"total"`
	TotalPages int              `json:"totalPages"`
	DataNotice string           `json:"dataNotice,omitempty"`
}

func (s *server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if s.accountService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "account service is not configured"})
		return
	}
	remainder := strings.Trim(strings.TrimPrefix(r.URL.Path, accountRoutePrefix), "/")
	if remainder == "" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "account collection only supports GET"})
			return
		}
		s.handleAccountList(w, r)
		return
	}
	parts := strings.Split(remainder, "/")
	accountID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(accountID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid account id"})
		return
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		s.handleAccountDetail(w, r, accountID)
	case len(parts) == 1 && r.Method == http.MethodPatch:
		s.handleAccountPatch(w, r, accountID)
	case len(parts) == 2 && r.Method == http.MethodGet && strings.EqualFold(parts[1], "videos"):
		s.handleAccountVideos(w, r, accountID)
	case len(parts) == 2 && r.Method == http.MethodGet && strings.EqualFold(parts[1], "comments"):
		s.handleAccountComments(w, r, accountID)
	case len(parts) == 2 && r.Method == http.MethodPost && strings.EqualFold(parts[1], "profile"):
		s.handleAccountProfileRefresh(w, r, accountID)
	case len(parts) == 2 && r.Method == http.MethodPost && strings.EqualFold(parts[1], "monitoring"):
		s.handleAccountMonitoring(w, r, accountID)
	case len(parts) == 3 && r.Method == http.MethodPost && strings.EqualFold(parts[1], "comments") && strings.EqualFold(parts[2], "backfill"):
		s.handleAccountCommentBackfill(w, r, accountID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "account method or route is not allowed"})
	}
}

func (s *server) handleAccountProfileRefresh(w http.ResponseWriter, r *http.Request, accountID string) {
	if s.accountService == nil || s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "account or ingest service is not configured"})
		return
	}
	query := accountQueryFromRequest(r)
	account, err := s.accountService.Get(r.Context(), query, accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	if account.IdentityStatus != insight.AccountIdentityVerified || strings.TrimSpace(account.ChannelUsername) == "" {
		writeAccountError(w, insight.ErrAccountIdentityIncomplete)
		return
	}
	profiles, err := s.ingestService.SearchWXFeedAccounts(r.Context(), ingest.SummaryQuery{
		HubUserID: query.HubUserID, DeviceCount: query.DeviceCount,
	}, "", account.ChannelUsername, "")
	if err != nil {
		writeAccountError(w, err)
		return
	}
	if len(profiles) == 0 {
		writeAccountError(w, errors.New("账号资料接口未返回匹配账号"))
		return
	}
	profile := profiles[0]
	if strings.TrimSpace(profile.Username) == "" {
		profile.Username = account.ChannelUsername
	}
	updated, err := s.accountService.UpsertObservedProfile(r.Context(), query, insight.AccountObservedProfile{
		Platform: insight.AccountPlatformWXChannels, ChannelUsername: profile.Username,
		ChannelName: profile.Nickname, Signature: profile.Signature, AvatarURL: profile.HeadURL,
		Region: profile.Region, Industry: profile.Industry, Tags: append([]string(nil), profile.Tags...),
		ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		writeAccountError(w, err)
		return
	}
	metricRows, dataNotice := s.loadAccountMetricRows(r, updated)
	item := s.buildAccountAPIItem(r, updated, metricRows[updated.ID])
	payload := map[string]any{"account": item, "monitoring": item.Monitoring}
	if dataNotice != "" {
		payload["dataNotice"] = dataNotice
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *server) handleAccountList(w http.ResponseWriter, r *http.Request) {
	query, err := parseAccountQuery(r)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	filters, err := parseAccountReadFilters(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if filters.status != "" || !filters.from.IsZero() || !filters.to.IsZero() {
		s.handleFilteredAccountList(w, r, query, filters)
		return
	}
	page, err := s.accountService.List(r.Context(), query)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	metricRows, dataNotice := s.loadAccountMetricRows(r, page.Items...)
	monitoringByAccount := s.accountMonitoringSummaries(r, page.Items)
	items := make([]accountAPIItem, 0, len(page.Items))
	for _, account := range page.Items {
		items = append(items, s.buildAccountAPIItemWithMonitoring(account, metricRows[account.ID], monitoringByAccount[account.ID]))
	}
	writeJSON(w, http.StatusOK, accountAPIPage{
		Items: items, Page: page.Page, PageSize: page.PageSize, Total: page.Total,
		TotalPages: page.TotalPages, DataNotice: dataNotice,
	})
}

type accountReadFilters struct {
	status   string
	from, to time.Time
}

func parseAccountReadFilters(r *http.Request) (accountReadFilters, error) {
	values := r.URL.Query()
	filters := accountReadFilters{status: strings.TrimSpace(values.Get("monitoringStatus"))}
	switch filters.status {
	case "", "enabled", "paused", "unmonitored":
	default:
		return filters, errors.New("unsupported account monitoringStatus")
	}
	var err error
	if filters.from, err = parseAccountTime(values.Get("from"), false); err != nil {
		return filters, errors.New("account from must be a valid date or RFC3339 time")
	}
	if filters.to, err = parseAccountTime(values.Get("to"), true); err != nil {
		return filters, errors.New("account to must be a valid date or RFC3339 time")
	}
	if !filters.from.IsZero() && !filters.to.IsZero() && filters.to.Before(filters.from) {
		return filters, errors.New("account to must not be before from")
	}
	return filters, nil
}

// Monitoring and observation filters belong to this joined read model, rather
// than the canonical account profile. Filter all matches before paginating.
func (s *server) handleFilteredAccountList(w http.ResponseWriter, r *http.Request, query insight.AccountQuery, filters accountReadFilters) {
	all := []insight.Account{}
	scan := query
	scan.Page, scan.PageSize = 1, 100
	for {
		page, err := s.accountService.List(r.Context(), scan)
		if err != nil {
			writeAccountError(w, err)
			return
		}
		all = append(all, page.Items...)
		if scan.Page >= page.TotalPages {
			break
		}
		scan.Page++
	}
	metrics, notice := s.loadAccountMetricRows(r, all...)
	dateBounded := !filters.from.IsZero() || !filters.to.IsZero()
	if dateBounded && (s.dataCenterService == nil || notice != "") {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "最近采集筛选暂不可用，请稍后重试。"})
		return
	}
	monitoringByAccount := s.accountMonitoringSummaries(r, all)
	items := []accountAPIItem{}
	for _, account := range all {
		state := monitoringByAccount[account.ID]
		if filters.status != "" {
			if state["error"] != nil || s.monitoringService == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "监控状态筛选暂不可用，请检查监控服务后重试。"})
				return
			}
			status := "unmonitored"
			if state["enabled"] == true {
				status = "enabled"
			} else if count, _ := state["taskCount"].(int); count > 0 {
				status = "paused"
			}
			if status != filters.status {
				continue
			}
		}
		row := metrics[account.ID]
		if dateBounded && (row.VideoCount == nil || *row.VideoCount == 0 || row.ObservedAt.IsZero() ||
			(!filters.from.IsZero() && row.ObservedAt.Before(filters.from)) || (!filters.to.IsZero() && row.ObservedAt.After(filters.to))) {
			continue
		}
		items = append(items, s.buildAccountAPIItemWithMonitoring(account, row, state))
	}
	total := len(items)
	pages := max(1, (total+query.PageSize-1)/query.PageSize)
	page := min(query.Page, pages)
	start := min((page-1)*query.PageSize, total)
	end := min(start+query.PageSize, total)
	writeJSON(w, http.StatusOK, accountAPIPage{Items: items[start:end], Page: page, PageSize: query.PageSize, Total: total, TotalPages: pages, DataNotice: notice})
}

func (s *server) handleAccountDetail(w http.ResponseWriter, r *http.Request, accountID string) {
	query := accountQueryFromRequest(r)
	account, err := s.accountService.Get(r.Context(), query, accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	metricRows, dataNotice := s.loadAccountMetricRows(r, account)
	accountItem := s.buildAccountAPIItem(r, account, metricRows[account.ID])
	payload := map[string]any{
		"account":      accountItem,
		"monitoring":   accountItem.Monitoring,
		"videosPath":   accountRoutePrefix + "/" + url.PathEscape(account.ID) + "/videos",
		"commentsPath": accountRoutePrefix + "/" + url.PathEscape(account.ID) + "/comments",
	}
	if dataNotice != "" {
		payload["dataNotice"] = dataNotice
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *server) buildAccountAPIItem(r *http.Request, account insight.Account, row insight.DataCenterItem) accountAPIItem {
	return s.buildAccountAPIItemWithMonitoring(account, row, s.accountMonitoringSummary(r, account))
}

func (s *server) buildAccountAPIItemWithMonitoring(account insight.Account, row insight.DataCenterItem, monitoring map[string]any) accountAPIItem {
	item := accountAPIItem{Account: insight.CloneAccount(account), Monitoring: monitoring}
	if strings.TrimSpace(row.ID) == "" {
		return item
	}
	item.Metrics = &accountDerivedMetrics{
		VideoCount: cloneAccountMetricInt(row.VideoCount), Likes: cloneAccountMetricInt(row.Likes),
		Comments: cloneAccountMetricInt(row.Comments), Favorites: cloneAccountMetricInt(row.Favorites),
		Forwards: cloneAccountMetricInt(row.Forwards), RecentObservedAt: row.ObservedAt, DataBasis: row.DataBasis,
	}
	if row.VideoCount == nil || *row.VideoCount == 0 {
		item.Metrics.RecentObservedAt = time.Time{}
	}
	return item
}

func (s *server) accountMonitoringSummaries(r *http.Request, accounts []insight.Account) map[string]map[string]any {
	result := make(map[string]map[string]any, len(accounts))
	for _, account := range accounts {
		result[account.ID] = map[string]any{"enabled": false, "taskCount": 0, "tasks": []any{}}
	}
	if s.monitoringService == nil || len(accounts) == 0 {
		return result
	}
	hub := hubContextFromRequest(r)
	tasks, err := s.monitoringService.ListTasks(r.Context(), hub.HubUserID, false)
	if err != nil {
		for accountID := range result {
			result[accountID] = map[string]any{"enabled": false, "taskCount": 0, "tasks": []any{}, "error": err.Error()}
		}
		return result
	}
	for _, task := range tasks {
		for _, account := range accounts {
			matched := task.ScopeMode == monitoring.ScopeAllAccounts && account.IdentityStatus == insight.AccountIdentityVerified
			commentMode := ""
			for _, target := range task.Targets {
				if (target.TargetKind == monitoring.TargetKindAccount && target.AccountID == account.ID) ||
					(target.Source == ingest.RunSourceWXFeedList && strings.EqualFold(strings.TrimSpace(target.TargetKey), strings.TrimSpace(account.ChannelUsername))) {
					matched = true
					commentMode = target.CommentMode
					break
				}
			}
			if !matched {
				continue
			}
			entry := result[account.ID]
			items, _ := entry["tasks"].([]any)
			items = append(items, map[string]any{
				"taskId": task.ID, "taskName": task.Name, "status": task.Status,
				"taskType": task.TaskType, "commentMode": commentMode,
				"lastStatus": task.LastStatus, "lastRunAt": task.LastRunAt,
				"lastSuccessAt": task.LastSuccessAt, "lastError": task.LastError,
				"nextRunAt": task.NextRunAt, "intervalSeconds": int64(task.Interval / time.Second),
			})
			entry["tasks"] = items
			entry["taskCount"] = len(items)
			if task.Status == monitoring.StatusEnabled {
				entry["enabled"] = true
			}
		}
	}
	return result
}

type boundedAccountMetricReader interface {
	ListAccountMetricRows(context.Context, ingest.SummaryQuery, []string) ([]ingest.AccountMetricRow, error)
}

func (s *server) loadAccountMetricRows(r *http.Request, accounts ...insight.Account) (map[string]insight.DataCenterItem, string) {
	rows := map[string]insight.DataCenterItem{}
	if s.dataCenterService == nil {
		return rows, ""
	}
	hub := hubContextFromRequest(r)
	if reader, ok := s.ingestService.(boundedAccountMetricReader); ok && len(accounts) > 0 {
		usernames := make([]string, 0, len(accounts))
		for _, account := range accounts {
			if username := strings.TrimSpace(account.ChannelUsername); username != "" {
				usernames = append(usernames, username)
			}
		}
		metricItems, err := reader.ListAccountMetricRows(r.Context(), ingest.SummaryQuery{
			HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount,
		}, usernames)
		if err == nil {
			for _, account := range accounts {
				for _, metric := range metricItems {
					if !strings.EqualFold(strings.TrimSpace(account.ChannelUsername), strings.TrimSpace(metric.ChannelUsername)) {
						continue
					}
					videoCount := metric.VideoCount
					rows[account.ID] = insight.DataCenterItem{
						ID: account.ID, AccountID: account.ID, Entity: insight.DataCenterEntityAccounts,
						ChannelUsername: account.ChannelUsername, ChannelName: account.ChannelName,
						ObservedAt: metric.RecentObservedAt, VideoCount: &videoCount, Likes: ingest.MetricValue(metric.Likes, metric.MissingMetrics, ingest.MetricLike),
						Comments: ingest.MetricValue(metric.Comments, metric.MissingMetrics, ingest.MetricComment), Favorites: ingest.MetricValue(metric.Favorites, metric.MissingMetrics, ingest.MetricFavorite), Forwards: ingest.MetricValue(metric.Forwards, metric.MissingMetrics, ingest.MetricForward),
						DataBasis: "当前账户页的真实视频观察指标。",
					}
					break
				}
			}
			return rows, ""
		}
		if !errors.Is(err, ingest.ErrAccountMetricProviderUnavailable) {
			return rows, "账户派生指标暂时不可用：" + err.Error()
		}
	}
	items, err := s.dataCenterService.ListAccountRows(r.Context(), insight.DataCenterQuery{
		HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount, ContextSource: hub.ContextSource,
		Entity: insight.DataCenterEntityAccounts, Sort: "observed_at", Direction: "desc",
	})
	if err != nil {
		return rows, "账户派生指标暂时不可用：" + err.Error()
	}
	for _, item := range items {
		key := strings.TrimSpace(item.AccountID)
		if key == "" {
			key = strings.TrimSpace(item.ID)
		}
		if key != "" {
			rows[key] = item
		}
	}
	return rows, ""
}

func cloneAccountMetricInt(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func (s *server) handleAccountPatch(w http.ResponseWriter, r *http.Request, accountID string) {
	var patch insight.AccountProfilePatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	updated, err := s.accountService.UpdateManualProfile(r.Context(), accountQueryFromRequest(r), accountID, patch)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	metricRows, dataNotice := s.loadAccountMetricRows(r, updated)
	accountItem := s.buildAccountAPIItem(r, updated, metricRows[updated.ID])
	payload := map[string]any{"account": accountItem, "monitoring": accountItem.Monitoring}
	if dataNotice != "" {
		payload["dataNotice"] = dataNotice
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *server) handleAccountVideos(w http.ResponseWriter, r *http.Request, accountID string) {
	account, err := s.accountService.Get(r.Context(), accountQueryFromRequest(r), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	if account.IdentityStatus != insight.AccountIdentityVerified || strings.TrimSpace(account.ChannelUsername) == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": insight.ErrAccountIdentityIncomplete.Error()})
		return
	}
	if s.dataCenterService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "data center service is not configured"})
		return
	}
	query, err := parseDataCenterQuery(r, insight.DataCenterEntityVideos)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	sortKey := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortKey == "" {
		sortKey = "published_at"
	}
	direction := strings.TrimSpace(r.URL.Query().Get("direction"))
	if direction == "" {
		direction = "desc"
	}
	query.Account = account.ChannelUsername
	query.Sort, query.Direction = sortKey, direction
	dcPage, err := s.dataCenterService.Query(r.Context(), query)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId": account.ID,
		"items":     dcPage.Items, "page": dcPage.Page, "pageSize": dcPage.PageSize,
		"total": dcPage.Total, "totalPages": dcPage.TotalPages,
		"summary": dcPage.Summary, "asOf": dcPage.AsOf, "generatedAt": dcPage.GeneratedAt,
	})
}

func (s *server) handleAccountComments(w http.ResponseWriter, r *http.Request, accountID string) {
	account, err := s.accountService.Get(r.Context(), accountQueryFromRequest(r), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	if s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest service is not configured"})
		return
	}
	page, err := parseAccountPageParams(r)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	from, err := parseAccountTime(r.URL.Query().Get("from"), false)
	if err != nil {
		writeAccountError(w, errors.New("account comments from must be RFC3339 or YYYY-MM-DD"))
		return
	}
	to, err := parseAccountTime(r.URL.Query().Get("to"), true)
	if err != nil {
		writeAccountError(w, errors.New("account comments to must be RFC3339 or YYYY-MM-DD"))
		return
	}
	comments, err := s.ingestService.ListRawComments(r.Context(), ingest.SummaryQuery{
		HubUserID: accountQueryFromRequest(r).HubUserID, DeviceCount: accountQueryFromRequest(r).DeviceCount,
	}, ingest.RawCommentQuery{
		AccountID: account.ID, Keyword: r.URL.Query().Get("q"), From: from, To: to,
		Page: page.page, PageSize: page.pageSize,
	})
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (s *server) handleAccountMonitoring(w http.ResponseWriter, r *http.Request, accountID string) {
	if s.monitoringService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "monitoring service is not configured"})
		return
	}
	account, err := s.accountService.Get(r.Context(), accountQueryFromRequest(r), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	if account.IdentityStatus != insight.AccountIdentityVerified || strings.TrimSpace(account.ChannelUsername) == "" {
		writeAccountError(w, insight.ErrAccountIdentityIncomplete)
		return
	}
	var body struct {
		TaskID      string `json:"taskId"`
		Name        string `json:"name"`
		TaskType    string `json:"taskType"`
		CommentMode string `json:"commentMode"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	mode := strings.TrimSpace(body.CommentMode)
	if mode == "" {
		mode = monitoring.CommentModeIncremental
	}
	target := monitoring.TaskTarget{TargetKind: monitoring.TargetKindAccount, AccountID: account.ID, CommentMode: mode, Label: "account", DisplayName: account.ChannelName}
	if _, err := monitoring.NormalizeTaskTarget(target); err != nil {
		writeAccountError(w, err)
		return
	}
	hub := hubContextFromRequest(r)
	if strings.TrimSpace(body.TaskID) == "" {
		taskType := firstNonEmptyTaskType(body.TaskType, monitoring.TaskTypeVideoList)
		interval := monitoring.DefaultTaskInterval
		if taskType == monitoring.TaskTypeVideoList {
			interval = metricFreshnessWindow
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = "账号监控 · " + firstNonEmptyAccountText(account.ChannelName, account.ChannelUsername)
		}
		created, err := s.monitoringService.CreateTask(r.Context(), hub.HubUserID, monitoring.Task{
			Name: name, Status: monitoring.StatusEnabled, ScopeMode: monitoring.ScopeSelected,
			TaskType: taskType,
			Interval: interval, Targets: []monitoring.TaskTarget{target},
		})
		if err != nil {
			writeAccountError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"task": created, "accountId": account.ID})
		return
	}
	task, err := s.monitoringService.GetTask(r.Context(), hub.HubUserID, strings.TrimSpace(body.TaskID))
	if err != nil {
		writeAccountError(w, err)
		return
	}
	if task.ScopeMode != monitoring.ScopeSelected {
		writeAccountError(w, errors.New("all-saved-bindings task cannot add an account target"))
		return
	}
	for _, existing := range task.Targets {
		if existing.TargetKind == monitoring.TargetKindAccount && existing.AccountID == account.ID {
			writeAccountError(w, monitoring.ErrTaskAlreadyExists)
			return
		}
	}
	if requestedType := strings.ToLower(strings.TrimSpace(body.TaskType)); requestedType != "" && requestedType != task.TaskType {
		writeAccountError(w, errors.New("任务类型不能通过账户监控接口修改"))
		return
	}
	task.Targets = append(task.Targets, target)
	updated, err := s.monitoringService.UpdateTask(r.Context(), hub.HubUserID, task)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": updated, "accountId": account.ID})
}

func (s *server) handleAccountCommentBackfill(w http.ResponseWriter, r *http.Request, accountID string) {
	if s.accountMonitor == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "account monitor is not configured"})
		return
	}
	account, err := s.accountService.Get(r.Context(), accountQueryFromRequest(r), accountID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	if account.IdentityStatus != insight.AccountIdentityVerified || strings.TrimSpace(account.ChannelUsername) == "" {
		writeAccountError(w, insight.ErrAccountIdentityIncomplete)
		return
	}
	hub := hubContextFromRequest(r)
	key := hub.HubUserID + "\x00" + account.ID
	s.accountBackfillMu.Lock()
	if s.accountBackfills == nil {
		s.accountBackfills = map[string]bool{}
	}
	if s.accountBackfills[key] {
		s.accountBackfillMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "该账号已有评论回采正在运行"})
		return
	}
	s.accountBackfills[key] = true
	s.accountBackfillMu.Unlock()
	defer func() {
		s.accountBackfillMu.Lock()
		delete(s.accountBackfills, key)
		s.accountBackfillMu.Unlock()
	}()
	result, runErr := s.accountMonitor.RunTask(r.Context(), ingest.SummaryQuery{HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount}, monitoring.TaskTarget{
		TargetKind: monitoring.TargetKindAccount, AccountID: account.ID, CommentMode: monitoring.CommentModeFullBackfill,
	}, monitoring.TaskTypeCommentCollect)
	if runErr != nil {
		writeAccountError(w, runErr)
		return
	}
	refreshError := ""
	if s.readModelRefreshService != nil && strings.TrimSpace(result.Summary.HubUserID) != "" {
		_, refreshErr := s.readModelRefreshService.RefreshDashboard(r.Context(), insight.DashboardQuery{
			HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount, ContextSource: hub.ContextSource,
		}, "account-comment-backfill")
		if refreshErr != nil {
			refreshError = refreshErr.Error()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId": account.ID, "status": result.Status, "attempted": result.Attempted,
		"succeeded": result.Succeeded, "failed": result.Failed, "skipped": result.Skipped,
		"errors": result.Errors, "refreshError": refreshError,
	})
}

func (s *server) accountMonitoringSummary(r *http.Request, account insight.Account) map[string]any {
	return s.accountMonitoringSummaries(r, []insight.Account{account})[account.ID]
}

func firstNonEmptyTaskType(values ...string) string {
	for _, value := range values {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			if monitoring.IsSeparatedTaskType(value) {
				return value
			}
		}
	}
	return monitoring.TaskTypeVideoList
}

func parseAccountQuery(r *http.Request) (insight.AccountQuery, error) {
	query := accountQueryFromRequest(r)
	values := r.URL.Query()
	query.Keyword = values.Get("q")
	query.Region = values.Get("region")
	query.Industry = values.Get("industry")
	query.Source = values.Get("source")
	query.Sort = values.Get("sort")
	query.Direction = values.Get("direction")
	if raw := strings.TrimSpace(values.Get("priority")); raw != "" {
		priority, err := strconv.ParseBool(raw)
		if err != nil {
			return insight.AccountQuery{}, errors.New("account priority must be boolean")
		}
		query.Priority = &priority
	}
	page, pageSize, err := parseAccountPageParamsValues(values.Get("page"), values.Get("pageSize"))
	if err != nil {
		return insight.AccountQuery{}, err
	}
	query.Page, query.PageSize = page, pageSize
	return query, nil
}

func accountQueryFromRequest(r *http.Request) insight.AccountQuery {
	hub := hubContextFromRequest(r)
	return insight.AccountQuery{HubUserID: hub.HubUserID, DeviceCount: hub.DeviceCount, ContextSource: hub.ContextSource}
}

type accountPageParams struct{ page, pageSize int }

func parseAccountPageParams(r *http.Request) (accountPageParams, error) {
	page, pageSize, err := parseAccountPageParamsValues(r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"))
	return accountPageParams{page: page, pageSize: pageSize}, err
}

func parseAccountPageParamsValues(pageRaw, pageSizeRaw string) (int, int, error) {
	page, pageSize := 1, 20
	var err error
	if strings.TrimSpace(pageRaw) != "" {
		page, err = strconv.Atoi(strings.TrimSpace(pageRaw))
		if err != nil || page < 1 {
			return 0, 0, errors.New("account page must be a positive integer")
		}
	}
	if strings.TrimSpace(pageSizeRaw) != "" {
		pageSize, err = strconv.Atoi(strings.TrimSpace(pageSizeRaw))
		if err != nil || (pageSize != 20 && pageSize != 50 && pageSize != 100) {
			return 0, 0, errors.New("account pageSize must be 20, 50 or 100")
		}
	}
	return page, pageSize, nil
}

func parseAccountTime(raw string, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if len(raw) == len("2006-01-02") {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return time.Time{}, err
		}
		if endOfDay {
			return value.Add(24*time.Hour - time.Nanosecond), nil
		}
		return value, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func firstNonEmptyAccountText(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "未命名账号"
}

func writeAccountError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if err == nil {
		status = http.StatusBadRequest
		err = errors.New("unknown account error")
	}
	switch {
	case errors.Is(err, insight.ErrAccountNotFound):
		status = http.StatusNotFound
	case errors.Is(err, insight.ErrAccountServiceUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(err, insight.ErrAccountIdentityIncomplete):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, monitoring.ErrTaskAlreadyExists), errors.Is(err, monitoring.ErrTaskRunInProgress):
		status = http.StatusConflict
	case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "unsupported"), strings.Contains(err.Error(), "must"), strings.Contains(err.Error(), "cannot"):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
