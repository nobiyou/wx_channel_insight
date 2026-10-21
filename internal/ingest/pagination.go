package ingest

import (
	"context"
	"encoding/json"
	"fmt"
)

type FeedPageTransport interface {
	SearchVideosPage(context.Context, string, string) (json.RawMessage, error)
	FetchFeedListPage(context.Context, string, string) (json.RawMessage, error)
}

func (c WXFeedListClient) collectPages(ctx context.Context, target string, search bool) (WXFeedListResult, error) {
	result := WXFeedListResult{Username: target, NextMarker: c.StartMarker}
	limit := c.MaxPages
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	seenCursors := map[string]bool{}
	seenItems := map[string]bool{}
	fetchPage := c.FetchPage
	if search {
		fetchPage = c.SearchVideosPage
	}
	appendItems := func(items []WXFeedListItem) {
		for _, item := range items {
			if !seenItems[item.ID] {
				seenItems[item.ID] = true
				result.Items = append(result.Items, item)
			}
		}
	}
	// A large historical backfill must not postpone new videos and changing
	// metrics until its final page. Keep its cursor while refreshing the head.
	if c.StartMarker != "" {
		head, err := fetchPage(ctx, target, "")
		if err != nil {
			return result, err
		}
		appendItems(head.Items)
		result.refreshedHeadItems = len(result.Items)
		result.refreshedHeadPages = 1
		result.PageCount++
	}
	for result.PageCount-result.refreshedHeadPages < limit {
		cursor := result.NextMarker
		if seenCursors[cursor] {
			result.Notice = "上游重复返回分页游标，已停止本轮采集；已保存结果，下次从断点重试。"
			break
		}
		seenCursors[cursor] = true
		page, err := fetchPage(ctx, target, cursor)
		if err != nil {
			if result.PageCount == 0 && len(result.Items) == 0 {
				return result, err
			}
			result.Notice = fmt.Sprintf("续页采集失败，已保留 %d 页结果，下次从断点重试：%v", result.PageCount, err)
			break
		}
		result.PageCount++
		appendItems(page.Items)
		result.NextMarker = page.NextMarker
		if result.NextMarker == "" {
			result.Complete = true
			break
		}
	}
	if !result.Complete && result.Notice == "" {
		result.Notice = fmt.Sprintf("本轮已采集 %d 页，仍有后续分页，下次从断点继续。", result.PageCount)
	}
	return result, nil
}
