package insight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var ErrOperationsStore = errors.New("智能运营需要 PostgreSQL 持久化存储，请先配置数据库")

const operationsRuleVersion = "operations-v2"
const operationsAIInputVersion = "operations-evidence-v3"
const operationsQueueLimit = 1000
const operationsRecentAge = 30 * 24 * time.Hour

// Implemented by the curation repository so decisions and their audit record
// commit in one transaction under the same workspace lock as manual edits.
type OperationsMutation func(*OperationsState, map[string]VideoCuration) ([]VideoCuration, error)
type OperationsRepository interface {
	LoadOperations(context.Context, string) (OperationsState, error)
	MutateOperations(context.Context, string, OperationsMutation) error
}
type AtomicCurationRepository interface {
	MutateCurations(context.Context, string, func(map[string]VideoCuration) ([]VideoCuration, error)) error
}
type OperationsState struct {
	RuleVersion string               `json:"ruleVersion"`
	Runs        []OperationsRun      `json:"runs"`
	Proposals   []OperationsProposal `json:"proposals"`
}
type OperationsQuality struct {
	Total     int `json:"total"`
	Fresh     int `json:"fresh"`
	Stale     int `json:"stale"`
	Missing   int `json:"missing"`
	Anomalous int `json:"anomalous"`
	Protected int `json:"protected"`
}
type OperationsRun struct {
	ID              string            `json:"id"`
	Scope           string            `json:"scope"`
	Trigger         string            `json:"trigger,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	RuleVersion     string            `json:"ruleVersion"`
	Fingerprint     string            `json:"fingerprint"`
	AIFingerprint   string            `json:"aiFingerprint"`
	EvidenceIDs     []string          `json:"evidenceIds"`
	Quality         OperationsQuality `json:"quality"`
	Proposed        int               `json:"proposed"`
	CapacityReached bool              `json:"capacityReached"`
	AIStatus        string            `json:"aiStatus"`
	AIStartedAt     time.Time         `json:"aiStartedAt,omitempty"`
	AIReason        string            `json:"aiReason"`
	AIAnalysisID    string            `json:"aiAnalysisId,omitempty"`
	AIModel         string            `json:"aiModel,omitempty"`
	AIResult        json.RawMessage   `json:"aiResult,omitempty"`
}
type OperationsEvent struct {
	Action  string    `json:"action"`
	Status  string    `json:"status"`
	Message string    `json:"message"`
	Actor   string    `json:"actor"`
	At      time.Time `json:"at"`
}
type OperationsProposal struct {
	RuleVersion  string            `json:"ruleVersion"`
	ID           string            `json:"id"`
	RunID        string            `json:"runId"`
	VideoID      string            `json:"videoId"`
	ObjectID     string            `json:"objectId"`
	Title        string            `json:"title"`
	Account      string            `json:"account"`
	Keywords     []string          `json:"keywords"`
	Action       string            `json:"action"`
	Reason       string            `json:"reason"`
	Evidence     string            `json:"evidence"`
	ObservedAt   time.Time         `json:"observedAt"`
	PublishedAt  time.Time         `json:"publishedAt"`
	GrowthWindow VideoGrowthWindow `json:"growthWindow"`
	Performance  VideoPerformance  `json:"performance"`
	Before       VideoCuration     `json:"before"`
	After        *VideoCuration    `json:"after,omitempty"`
	Status       string            `json:"status"`
	Message      string            `json:"message"`
	CreatedAt    time.Time         `json:"createdAt"`
	Events       []OperationsEvent `json:"events"`
}
type OperationsDecision struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func operationsHash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func operationsCuration(c VideoCuration, id string) VideoCuration {
	c.VideoID = id
	// PostgreSQL may scan the same instant in the host's local timezone.
	// Version equality compares the instant at database precision, not its text.
	c.UpdatedAt = c.UpdatedAt.UTC().Truncate(time.Microsecond)
	if c.Status == "" {
		c.Status = "unreviewed"
	}
	if c.Tags == nil {
		c.Tags = []string{}
	}
	return c
}
func operationsCurationEqual(a, b VideoCuration, id string) bool {
	return operationsHash(operationsCuration(a, id)) == operationsHash(operationsCuration(b, id))
}

func (s *DataCenterService) operationsRepo() (OperationsRepository, error) {
	r, ok := s.curations.(OperationsRepository)
	if !ok {
		return nil, ErrOperationsStore
	}
	return r, nil
}
func (s *DataCenterService) Operations(ctx context.Context, q DataCenterQuery) (OperationsState, error) {
	repo, err := s.operationsRepo()
	if err != nil {
		return OperationsState{}, err
	}
	workspace, _, err := s.resolveContext(ctx, q.HubUserID, q.DeviceCount, q.ContextSource)
	if err != nil {
		return OperationsState{}, err
	}
	state, err := repo.LoadOperations(ctx, workspace)
	if err != nil {
		return OperationsState{}, err
	}
	// Opening the page must not expose executable advice from retired rules.
	for _, p := range state.Proposals {
		if p.Status == "pending" && p.RuleVersion != operationsRuleVersion {
			err = repo.MutateOperations(ctx, workspace, func(current *OperationsState, _ map[string]VideoCuration) ([]VideoCuration, error) {
				retireOperationsRules(current, s.now().UTC())
				state = *current
				return nil, nil
			})
			break
		}
	}
	normalizeOperationsState(&state)
	return state, err
}
func normalizeOperationsState(state *OperationsState) {
	state.RuleVersion = operationsRuleVersion
	if state.Runs == nil {
		state.Runs = []OperationsRun{}
	}
	if state.Proposals == nil {
		state.Proposals = []OperationsProposal{}
	}
}
func retireOperationsRules(state *OperationsState, now time.Time) {
	for i := range state.Proposals {
		p := &state.Proposals[i]
		if p.Status == "pending" && p.RuleVersion != operationsRuleVersion {
			p.Status = "stale"
			proposalEvent(p, "recheck", "stale", "运营规则已升级，旧建议失效；重新巡检区分近期机会与历史案例", "operations-agent", now)
		}
	}
	normalizeOperationsState(state)
}
func (s *DataCenterService) operationsRows(ctx context.Context, q DataCenterQuery) ([]DataCenterItem, error) {
	// No client-provided date or filtering may turn an historical snapshot into
	// actionable evidence. Baselines remain owned by the normal video evaluator.
	q = DataCenterQuery{HubUserID: q.HubUserID, DeviceCount: q.DeviceCount, ContextSource: q.ContextSource, Entity: DataCenterEntityVideos, MonitorKeyword: q.MonitorKeyword, Account: q.Account, Curation: "all", Refresh: true, allItems: true}
	page, err := s.Query(ctx, q)
	return page.Items, err
}
func operationsAction(row DataCenterItem, now time.Time) (string, string) {
	p := row.Performance
	validPublication := !row.PublishedAt.IsZero() && !row.PublishedAt.After(now) && !row.PublishedAt.After(row.ObservedAt)
	recent := validPublication && now.Sub(row.PublishedAt) <= operationsRecentAge
	focus := row.Curation.Status == "selected" || row.Curation.Status == "watching"
	// Collection recency cannot substitute for publication recency. Untracked
	// archives must not manufacture an urgent refresh backlog.
	if !recent && !focus {
		if !validPublication || p.Interactions == nil || p.Freshness == "unknown" || row.GrowthStatus == "counter_correction" || row.GrowthStatus == "invalid" {
			return "", ""
		}
	} else {
		if p.Interactions == nil {
			return "refresh", "互动指标未采全，补齐后再评估，不按零值淘汰"
		}
		if p.Freshness != "fresh" {
			return "refresh", "指标已过期或时间不可信，先补采再判断表现"
		}
		if row.GrowthStatus == "counter_correction" || row.GrowthStatus == "invalid" {
			return "refresh", "计数发生回调或异常，需复核上游数据"
		}
	}
	if row.Curation.Status != "" && row.Curation.Status != "unreviewed" || row.Curation.UpdatedBy != "" || row.Curation.Note != "" || len(row.Curation.Tags) > 0 {
		return "", ""
	}
	if !recent {
		if operationsOldGrowth(row, now) {
			return "watching", fmt.Sprintf("老视频增长线索：近 72 小时内的两次可比采集新增 %d 次互动（%.1f 次/小时），增量达到基线 1%% 且至少 100 次；建议观察，尚不能认定持续走热", *p.GrowthInteractions, *p.GrowthPerHour)
		}
		if reason := operationsStandoutReason(p); reason != "" {
			return "historical", reason + "；发布已超过 30 天，仅作为历史复盘案例，累计高互动不能证明当前热门"
		}
		return "", ""
	}
	if reason := operationsStandoutReason(p); reason != "" {
		return "selected", "近 30 天发布，指标在 24 小时内更新；" + reason + "；建议纳入近期精选，累计表现不代表持续增长"
	}
	if p.Tier == "underperforming" {
		return "excluded", p.Reason + "；近 30 天发布且已满 3 天、历史可比，建议移出有效内容池（保留原始数据）"
	}
	if p.GrowthPerHour != nil && *p.GrowthPerHour > 0 {
		return "watching", fmt.Sprintf("近 30 天发布，可比窗口每小时增加 %.1f 次互动，建议继续观察", *p.GrowthPerHour)
	}
	return "", ""
}
func operationsStandoutReason(p VideoPerformance) string {
	if p.Tier == "standout" {
		return p.Reason
	}
	if p.Interactions != nil && *p.Interactions >= 100 {
		for _, rank := range p.KeywordRanks {
			if rank.State == "available" && rank.Percentile != nil && *rank.Percentile >= 90 && rank.SampleCount >= 10 {
				return fmt.Sprintf("关键词「%s」累计互动位于样本内前 10%%，共 %d 条完整样本", rank.Keyword, rank.SampleCount)
			}
		}
	}
	return ""
}
func operationsOldGrowth(row DataCenterItem, now time.Time) bool {
	p, w := row.Performance, row.GrowthWindow
	span := w.LatestObservedAt.Sub(w.BaselineObservedAt)
	if p.Freshness != "fresh" || !hasComparableInteractionCounts(row.GrowthStatus) || p.Interactions == nil || p.GrowthInteractions == nil || p.GrowthPerHour == nil || span < 10*time.Minute || span > 72*time.Hour || w.BaselineObservedAt.Before(now.Add(-72*time.Hour)) || !w.LatestObservedAt.Equal(row.ObservedAt) {
		return false
	}
	delta := *p.GrowthInteractions
	baseline := *p.Interactions - delta
	return delta >= 100 && baseline >= 0 && float64(delta) >= float64(baseline)*0.01
}
func operationsEvidence(row DataCenterItem) string {
	p := row.Performance
	// Heat naturally changes with wall time; hash its state, not the time-decayed
	// score. Counts, observation time and peer baseline still invalidate advice.
	p.RecentHeat.Score = nil
	return operationsHash(struct {
		Version                 string
		ID                      string
		ObservedAt, PublishedAt time.Time
		Growth                  string
		Counters                [4]*int
		P                       VideoPerformance
	}{operationsRuleVersion, row.ID, row.ObservedAt, row.PublishedAt, row.GrowthStatus, [4]*int{row.Likes, row.Comments, row.Favorites, row.Forwards}, p})
}
func operationsPriority(row DataCenterItem, now time.Time) float64 {
	action, _ := operationsAction(row, now)
	value := map[string]float64{"selected": 5e12, "watching": 4e12, "excluded": 3e12, "refresh": 2e12, "historical": 1e12}[action]
	if row.Performance.Interactions != nil {
		value += math.Log1p(math.Max(0, float64(*row.Performance.Interactions)))
	}
	if row.Curation.Status == "selected" || row.Curation.Status == "watching" {
		value += 1e11
	}
	if !row.PublishedAt.IsZero() && !row.PublishedAt.After(now) {
		value += math.Max(0, 720-now.Sub(row.PublishedAt).Hours()) * 1e6
	}
	return value
}

func (s *DataCenterService) InspectOperations(ctx context.Context, q DataCenterQuery) (OperationsState, bool, error) {
	return s.inspectOperations(ctx, q, "manual")
}

func (s *DataCenterService) InspectOperationsAfterCollection(ctx context.Context, q DataCenterQuery) (OperationsState, bool, error) {
	return s.inspectOperations(ctx, q, "collection")
}

func (s *DataCenterService) inspectOperations(ctx context.Context, q DataCenterQuery, trigger string) (OperationsState, bool, error) {
	repo, err := s.operationsRepo()
	if err != nil {
		return OperationsState{}, false, err
	}
	workspace, _, err := s.resolveContext(ctx, q.HubUserID, q.DeviceCount, q.ContextSource)
	if err != nil {
		return OperationsState{}, false, err
	}
	rows, err := s.operationsRows(ctx, q)
	if err != nil {
		return OperationsState{}, false, err
	}
	if trigger == "collection" && len(rows) == 0 {
		state, err := s.Operations(ctx, q)
		return state, true, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	priorities := make(map[string]float64, len(rows))
	for _, row := range rows {
		priorities[row.ID] = operationsPriority(row, now)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := priorities[rows[i].ID], priorities[rows[j].ID]
		if a == b {
			return rows[i].ID < rows[j].ID
		}
		return a > b
	})
	quality := OperationsQuality{Total: len(rows)}
	fingerprints := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Performance.Freshness == "fresh" {
			quality.Fresh++
		} else {
			quality.Stale++
		}
		if row.Performance.Interactions == nil {
			quality.Missing++
		}
		if row.GrowthStatus == "counter_correction" {
			quality.Anomalous++
		}
		if row.Curation.UpdatedBy != "" || row.Curation.Status != "unreviewed" || row.Curation.Note != "" || len(row.Curation.Tags) > 0 {
			quality.Protected++
		}
		fingerprints = append(fingerprints, operationsEvidence(row)+operationsHash(row.Curation))
	}
	scope := "全部视频"
	if q.MonitorKeyword != "" {
		scope = "关键词：" + q.MonitorKeyword
	}
	if q.Account != "" {
		scope += " · 账号：" + q.Account
	}
	fingerprint := operationsHash(struct {
		Scope, Version, AIInputVersion string
		Facts                          []string
	}{scope, operationsRuleVersion, operationsAIInputVersion, fingerprints})
	run := OperationsRun{ID: "opr_" + operationsHash([]string{workspace, fingerprint, now.Format(time.RFC3339Nano)})[:24], Scope: scope, Trigger: trigger, CreatedAt: now, RuleVersion: operationsRuleVersion, Fingerprint: fingerprint, Quality: quality, AIStatus: "not_requested", AIReason: "规则巡检；可启用 AI 补充研判"}
	reused := false
	var result OperationsState
	err = repo.MutateOperations(ctx, workspace, func(state *OperationsState, current map[string]VideoCuration) ([]VideoCuration, error) {
		retireOperationsRules(state, now)
		if len(state.Runs) > 0 && state.Runs[0].Fingerprint == fingerprint && now.Sub(state.Runs[0].CreatedAt) < time.Minute {
			reused = true
			result = *state
			return nil, nil
		}
		known := map[string]bool{}
		byID := map[string]DataCenterItem{}
		for _, row := range rows {
			byID[row.ID] = row
		}
		for i := range state.Proposals {
			p := &state.Proposals[i]
			if p.Status != "pending" {
				continue
			}
			row, ok := byID[p.VideoID]
			if !ok {
				continue
			}
			row.Curation = operationsCuration(current[p.VideoID], p.VideoID)
			next, _ := operationsAction(row, now)
			if next != p.Action || operationsEvidence(row) != p.Evidence || !operationsCurationEqual(row.Curation, p.Before, p.VideoID) {
				p.Status = "stale"
				proposalEvent(p, "recheck", "stale", "新证据或人工修改使旧建议失效", "operations-agent", now)
			}
		}
		for _, p := range state.Proposals {
			known[p.ID] = true
		}
		candidates := []OperationsProposal{}
		for _, row := range rows {
			row.Curation = operationsCuration(current[row.ID], row.ID)
			action, reason := operationsAction(row, now)
			if action == "" {
				continue
			}
			evidence := operationsEvidence(row)
			id := "opp_" + operationsHash([]string{operationsRuleVersion, row.ID, action, evidence, operationsHash(row.Curation)})[:28]
			if known[id] {
				continue
			}
			message := "等待人工确认"
			if action == "historical" {
				message = "仅供历史复盘，不参与近期精选确认"
			}
			candidates = append(candidates, OperationsProposal{RuleVersion: operationsRuleVersion, ID: id, RunID: run.ID, VideoID: row.ID, ObjectID: row.ObjectID, Title: row.Title, Account: row.ChannelName, Keywords: row.SourceKeywords, Action: action, Reason: reason, Evidence: evidence, ObservedAt: row.ObservedAt, PublishedAt: row.PublishedAt, GrowthWindow: row.GrowthWindow, Performance: row.Performance, Before: row.Curation, Status: "pending", Message: message, CreatedAt: now, Events: []OperationsEvent{}})
		}
		// Reserve room for actionable opportunities so a large stale library
		// cannot starve the fresh content queue. Unused slots go to either kind.
		counts := map[string]int{}
		limits := map[string]int{"opportunity": 8, "refresh": 8, "historical": 4}
		chosen := map[string]bool{}
		for pass := 0; pass < 2; pass++ {
			for _, candidate := range candidates {
				kind := "opportunity"
				if candidate.Action == "refresh" || candidate.Action == "historical" {
					kind = candidate.Action
				}
				if chosen[candidate.ID] || pass == 0 && counts[kind] >= limits[kind] {
					continue
				}
				if run.Proposed >= 20 {
					break
				}
				if len(state.Proposals) >= operationsQueueLimit {
					run.CapacityReached = true
					break
				}
				state.Proposals = append(state.Proposals, candidate)
				chosen[candidate.ID] = true
				counts[kind]++
				run.Proposed++
			}
		}
		// AI reuse follows the exact candidate evidence, not just a global
		// database snapshot: successive batches may contain different videos.
		aiEvidence := []string{}
		for _, p := range state.Proposals {
			if len(run.EvidenceIDs) >= 20 {
				break
			}
			if p.Status != "pending" {
				continue
			}
			if _, ok := byID[p.VideoID]; !ok {
				continue
			}
			if run.Proposed > 0 && !chosen[p.ID] {
				continue
			}
			run.EvidenceIDs = append(run.EvidenceIDs, p.ID)
			aiEvidence = append(aiEvidence, operationsHash(struct {
				ID, Title, Reason string
				Performance       VideoPerformance
			}{p.ID, p.Title, p.Reason, p.Performance}))
		}
		run.AIFingerprint = operationsHash(struct {
			Scope, Version, AIInputVersion string
			Quality                        OperationsQuality
			Evidence                       []string
		}{scope, operationsRuleVersion, operationsAIInputVersion, quality, aiEvidence})
		for _, old := range state.Runs {
			if old.AIFingerprint == run.AIFingerprint && old.AIStatus != "not_requested" && old.AIStatus != "running" {
				run.AIStatus, run.AIReason, run.AIAnalysisID, run.AIModel, run.AIResult = old.AIStatus, old.AIReason, old.AIAnalysisID, old.AIModel, old.AIResult
				break
			}
		}
		state.Runs = append([]OperationsRun{run}, state.Runs...)
		if len(state.Runs) > 20 {
			state.Runs = state.Runs[:20]
		}
		result = *state
		return nil, nil
	})
	return result, reused, err
}
func operationsIDs(ids []string) ([]string, error) {
	if len(ids) < 1 || len(ids) > 50 {
		return nil, errors.New("每次请选择 1–50 条建议")
	}
	result := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result, nil
}
func proposalEvent(p *OperationsProposal, action, status, message, actor string, at time.Time) {
	p.Message = message
	p.Events = append(p.Events, OperationsEvent{Action: action, Status: status, Message: message, Actor: actor, At: at})
}
func (s *DataCenterService) DecideOperations(ctx context.Context, q DataCenterQuery, action string, ids []string, actor string) ([]OperationsDecision, error) {
	if action != "apply" && action != "dismiss" && action != "undo" {
		return nil, errors.New("不支持的建议操作")
	}
	ids, err := operationsIDs(ids)
	if err != nil {
		return nil, err
	}
	repo, err := s.operationsRepo()
	if err != nil {
		return nil, err
	}
	workspace, _, err := s.resolveContext(ctx, q.HubUserID, q.DeviceCount, q.ContextSource)
	if err != nil {
		return nil, err
	}
	rowsByID := map[string]DataCenterItem{}
	if action == "apply" {
		// Reassess the full current workspace: the candidate's own peer cohort is
		// identical to the one used at inspection, independent of the selected tab.
		rows, err := s.operationsRows(ctx, DataCenterQuery{HubUserID: q.HubUserID, DeviceCount: q.DeviceCount, ContextSource: q.ContextSource})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			rowsByID[row.ID] = row
		}
	}
	result := []OperationsDecision{}
	now := s.now().UTC().Truncate(time.Microsecond)
	err = repo.MutateOperations(ctx, workspace, func(state *OperationsState, current map[string]VideoCuration) ([]VideoCuration, error) {
		retireOperationsRules(state, now)
		indexes := map[string]int{}
		for i, p := range state.Proposals {
			indexes[p.ID] = i
		}
		for _, id := range ids {
			if _, ok := indexes[id]; !ok {
				return nil, fmt.Errorf("%w: proposal %s", ErrDataCenterNotFound, id)
			}
		}
		writes := []VideoCuration{}
		for _, id := range ids {
			p := &state.Proposals[indexes[id]]
			status, message := p.Status, p.Message
			before := operationsCuration(current[p.VideoID], p.VideoID)
			switch {
			case action == "undo" && p.Status == "dismissed" && p.RuleVersion != operationsRuleVersion:
				p.Status, status, message = "stale", "stale", "运营规则已升级，请重新巡检；旧建议不能恢复执行"
				proposalEvent(p, action, status, message, actor, now)
			case action == "undo" && p.Status == "dismissed":
				p.Status, status, message = "pending", "pending", "已撤销忽略，确认前仍会复核最新证据"
				proposalEvent(p, action, status, message, actor, now)
			case action == "dismiss" && p.Status == "pending":
				p.Status, status, message = "dismissed", "dismissed", "已忽略，本份证据不会重复建议"
				proposalEvent(p, action, status, message, actor, now)
			case action == "apply" && p.Status == "pending" && p.Action == "refresh":
				status, message = "needs_refresh", "请通过补采操作更新指标，再重新巡检"
			case action == "apply" && p.Status == "pending" && p.Action == "historical":
				status, message = "reference_only", "历史案例仅供复盘，不自动改变近期精选状态"
			case action == "apply" && p.Status == "pending":
				row, exists := rowsByID[p.VideoID]
				row.Curation = before
				next, _ := operationsAction(row, now)
				if !exists || next != p.Action || operationsEvidence(row) != p.Evidence || !operationsCurationEqual(before, p.Before, p.VideoID) {
					p.Status, status, message = "stale", "stale", "证据或人工标记已变化，请重新巡检后确认"
				} else {
					after := before
					after.WorkspaceID = workspace
					after.Status = p.Action
					after.UpdatedAt = now
					after.UpdatedBy = "operations-agent:" + actor
					p.After = &after
					p.Status, status, message = "applied", "applied", "已按确认更新内容池，可撤销"
					current[p.VideoID] = after
					writes = append(writes, after)
				}
				proposalEvent(p, action, status, message, actor, now)
			case action == "undo" && p.Status == "applied" && p.Action != "refresh" && p.After != nil:
				if !operationsCurationEqual(before, *p.After, p.VideoID) {
					status, message = "conflict", "存在后续人工修改，未覆盖；请在视频详情中处理"
				} else {
					restored := p.Before
					restored.WorkspaceID = workspace
					restored.UpdatedAt = now
					restored.UpdatedBy = actor
					writes = append(writes, restored)
					current[p.VideoID] = restored
					p.Status, status, message = "undone", "undone", "已恢复执行前的状态、标签与备注"
				}
				proposalEvent(p, action, status, message, actor, now)
			}
			result = append(result, OperationsDecision{ID: id, Status: status, Message: message})
		}
		return writes, nil
	})
	return result, err
}

func (s *DataCenterService) SaveOperationsAI(ctx context.Context, q DataCenterQuery, runID, status, reason, analysisID, model string, result json.RawMessage) error {
	repo, err := s.operationsRepo()
	if err != nil {
		return err
	}
	workspace, _, err := s.resolveContext(ctx, q.HubUserID, q.DeviceCount, q.ContextSource)
	if err != nil {
		return err
	}
	return repo.MutateOperations(ctx, workspace, func(state *OperationsState, _ map[string]VideoCuration) ([]VideoCuration, error) {
		for i := range state.Runs {
			r := &state.Runs[i]
			if r.ID == runID {
				r.AIStatus = status
				r.AIReason = reason
				r.AIAnalysisID = analysisID
				r.AIModel = model
				r.AIResult = nil
				if status == "completed" {
					r.AIResult = result
				}
				return nil, nil
			}
		}
		return nil, ErrDataCenterNotFound
	})
}

func (s *DataCenterService) ClaimOperationsAI(ctx context.Context, q DataCenterQuery, runID string) (bool, error) {
	repo, err := s.operationsRepo()
	if err != nil {
		return false, err
	}
	workspace, _, err := s.resolveContext(ctx, q.HubUserID, q.DeviceCount, q.ContextSource)
	if err != nil {
		return false, err
	}
	claimed := false
	err = repo.MutateOperations(ctx, workspace, func(state *OperationsState, _ map[string]VideoCuration) ([]VideoCuration, error) {
		for i := range state.Runs {
			r := &state.Runs[i]
			if r.ID == runID {
				if r.AIStatus != "not_requested" && !(r.AIStatus == "running" && s.now().Sub(r.AIStartedAt) > 3*time.Minute) {
					return nil, nil
				}
				if len(r.EvidenceIDs) == 0 {
					r.AIStatus, r.AIReason = "insufficient_evidence", "本轮没有待研判建议，未调用 AI；可继续采集积累证据"
					return nil, nil
				}
				// The same evidence can occur in different manual/automatic runs.
				// Claim and reuse under the workspace transaction lock, not in memory.
				for _, other := range state.Runs {
					if other.ID == r.ID || other.AIFingerprint != r.AIFingerprint || r.AIFingerprint == "" {
						continue
					}
					if other.AIStatus == "running" && s.now().Sub(other.AIStartedAt) <= 3*time.Minute {
						return nil, nil
					}
					if other.AIStatus != "" && other.AIStatus != "not_requested" && other.AIStatus != "running" {
						r.AIStatus, r.AIReason, r.AIAnalysisID, r.AIModel, r.AIResult = other.AIStatus, other.AIReason, other.AIAnalysisID, other.AIModel, other.AIResult
						return nil, nil
					}
				}
				if r.AIStatus == "not_requested" || r.AIStatus == "running" && s.now().Sub(r.AIStartedAt) > 3*time.Minute {
					claimed = true
					r.AIStatus = "running"
					r.AIStartedAt = s.now().UTC()
					r.AIReason = "正在补充 AI 研判，规则建议可先处理"
				}
				return nil, nil
			}
		}
		return nil, ErrDataCenterNotFound
	})
	return claimed, err
}

func (s *DataCenterService) OperationsRefreshTarget(ctx context.Context, q DataCenterQuery, id string) (OperationsProposal, error) {
	state, err := s.Operations(ctx, q)
	if err != nil {
		return OperationsProposal{}, err
	}
	for _, p := range state.Proposals {
		if p.ID == id && p.Action == "refresh" && p.Status == "pending" {
			return p, nil
		}
	}
	return OperationsProposal{}, ErrDataCenterNotFound
}
func (s *DataCenterService) RecordOperationsRefresh(ctx context.Context, q DataCenterQuery, id, status, message, actor string) error {
	repo, err := s.operationsRepo()
	if err != nil {
		return err
	}
	workspace, _, err := s.resolveContext(ctx, q.HubUserID, q.DeviceCount, q.ContextSource)
	if err != nil {
		return err
	}
	// A collector's success must also be backed by fresh, complete saved facts.
	complete := map[string]bool{}
	if status == "updated" || status == "fresh" {
		rows, err := s.operationsRows(ctx, q)
		if err != nil {
			return err
		}
		for _, row := range rows {
			complete[row.ID] = row.Performance.Freshness == "fresh" && row.Performance.Interactions != nil
		}
	}
	return repo.MutateOperations(ctx, workspace, func(state *OperationsState, _ map[string]VideoCuration) ([]VideoCuration, error) {
		for i := range state.Proposals {
			p := &state.Proposals[i]
			if p.ID != id {
				continue
			}
			if p.Action != "refresh" {
				return nil, errors.New("不是补采建议")
			}
			if complete[p.VideoID] && p.Status == "pending" {
				p.Status = "applied"
				message = "已验证新指标入库，请重新巡检生成运营建议"
			} else if status == "updated" || status == "fresh" {
				status = "incomplete"
				message = "采集已返回，完整指标仍待补齐；尚未改变内容状态"
			}
			proposalEvent(p, "refresh", status, strings.TrimSpace(message), actor, s.now().UTC().Truncate(time.Microsecond))
			return nil, nil
		}
		return nil, ErrDataCenterNotFound
	})
}
