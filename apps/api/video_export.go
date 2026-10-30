package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

var csvVideoLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func (s *server) handleVideoExport(w http.ResponseWriter, r *http.Request, query insight.DataCenterQuery) {
	var request struct {
		Scope string   `json:"scope"`
		IDs   []string `json:"ids"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "导出请求无效，请重新选择导出范围"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "导出请求必须是单个 JSON 对象"})
		return
	}
	if request.Scope != "filtered" && request.Scope != "selected" ||
		request.Scope == "selected" && len(request.IDs) == 0 ||
		request.Scope == "filtered" && len(request.IDs) != 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请选择导出全部筛选结果，或勾选要导出的视频"})
		return
	}
	items, err := s.dataCenterService.ExportVideos(r.Context(), query, request.IDs)
	if err != nil {
		writeDataCenterError(w, err)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	filename := "insight-videos-" + time.Now().UTC().Format("20060102T150405Z") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := writeVideoCSV(r.Context(), w, items); err != nil {
		// Abort a partial response so a truncated export cannot appear successful.
		log.Printf("video CSV response interrupted: %v", err)
		panic(http.ErrAbortHandler)
	}
}

func writeVideoCSV(ctx context.Context, output io.Writer, items []insight.DataCenterItem) error {
	if _, err := io.WriteString(output, "\ufeff"); err != nil {
		return err
	}
	writer := csv.NewWriter(output)
	writer.UseCRLF = true
	if err := writer.Write([]string{
		"视频ID", "标题", "账号名称", "账号标识", "来源关键词", "发布时间（北京时间）", "采集时间（北京时间）",
		"播放量", "点赞", "评论", "收藏", "转发", "总互动", "每小时新增互动", "历史增长率（%）", "增长状态", "增长依据", "观察次数",
		"表现分层", "表现依据", "运营标记", "标签", "备注", "增长观察起点（北京时间）", "增长观察终点（北京时间）",
	}); err != nil {
		return err
	}
	performanceLabels := map[string]string{"standout": "表现突出", "typical": "常规表现", "underperforming": "低表现待复核", "insufficient_data": "待积累依据"}
	curationLabels := map[string]string{"unreviewed": "未处理", "selected": "已精选", "watching": "待观察", "excluded": "已排除"}
	growthLabels := map[string]string{"available": "可比较", "zero_baseline": "基线为零，百分比不可用", "metrics_unavailable": "指标缺失", "counter_correction": "计数修正或异常", "insufficient_history": "待积累历史"}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := []string{
			csvSafeText(item.ObjectID), csvSafeText(item.Title), csvSafeText(item.ChannelName), csvSafeText(item.ChannelUsername), csvSafeText(strings.Join(item.SourceKeywords, "；")),
			csvVideoTime(item.PublishedAt), csvVideoTime(item.ObservedAt), csvVideoInt(item.Views), csvVideoInt(item.Likes), csvVideoInt(item.Comments), csvVideoInt(item.Favorites), csvVideoInt(item.Forwards),
			csvVideoInt(item.Performance.Interactions), csvVideoFloat(item.Performance.GrowthPerHour), csvVideoFloat(item.GrowthRate), growthLabels[item.GrowthStatus], csvSafeText(item.GrowthBasis), strconv.Itoa(item.ObservationCount),
			performanceLabels[item.Performance.Tier], csvSafeText(item.Performance.Reason), curationLabels[item.Curation.Status], csvSafeText(strings.Join(item.Curation.Tags, "；")), csvSafeText(item.Curation.Note),
			csvVideoTime(item.GrowthWindow.BaselineObservedAt), csvVideoTime(item.GrowthWindow.LatestObservedAt),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// Untrusted titles/notes must not become formulas. Long numeric identifiers
// also need a text prefix to survive Excel's 15-digit numeric precision limit.
func csvSafeText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) || strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") {
		return "'" + value
	}
	if len(value) >= 15 || len(value) > 1 && value[0] == '0' {
		if strings.Trim(value, "0123456789") == "" {
			return "'" + value
		}
	}
	return value
}

func csvVideoInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func csvVideoFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func csvVideoTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(csvVideoLocation).Format("2006-01-02 15:04:05")
}
