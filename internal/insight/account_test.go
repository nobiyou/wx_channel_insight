package insight_test

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
)

func TestAccountServicePreservesManualProfileAcrossObservedRefresh(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	service := insight.NewAccountService(insight.AccountServiceConfig{
		Repository: insightdemo.NewAccountRepository(),
		Now:        func() time.Time { return now },
	})
	ctx := context.Background()
	query := insight.AccountQuery{HubUserID: "hub-user"}

	account, err := service.UpsertObservedProfile(ctx, query, insight.AccountObservedProfile{
		Platform:        insight.AccountPlatformWXChannels,
		ChannelUsername: "finder-001",
		ChannelName:     "原始昵称",
		Signature:       "原始简介",
		AvatarURL:       "https://img.example/old.png",
		ObservedAt:      now,
	})
	if err != nil {
		t.Fatalf("upsert observed profile: %v", err)
	}
	region := "华东"
	industry := "教育"
	note := "重点跟进"
	priority := true
	updated, err := service.UpdateManualProfile(ctx, query, account.ID, insight.AccountProfilePatch{
		Region:     &region,
		Industry:   &industry,
		Tags:       []string{"重点", "教育"},
		Note:       &note,
		IsPriority: &priority,
	})
	if err != nil {
		t.Fatalf("update manual profile: %v", err)
	}
	if updated.Region != region || updated.Industry != industry || !updated.IsPriority || len(updated.Tags) != 2 {
		t.Fatalf("manual profile was not saved: %+v", updated)
	}

	refreshed, err := service.UpsertObservedProfile(ctx, query, insight.AccountObservedProfile{
		Platform:        insight.AccountPlatformWXChannels,
		ChannelUsername: "finder-001",
		ChannelName:     "新昵称",
		Signature:       "新简介",
		AvatarURL:       "https://img.example/new.png",
		ObservedAt:      now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("refresh observed profile: %v", err)
	}
	if refreshed.ChannelName != "新昵称" || refreshed.Signature != "新简介" || refreshed.AvatarURL == "" {
		t.Fatalf("observed fields were not refreshed: %+v", refreshed)
	}
	if refreshed.Region != region || refreshed.Industry != industry || refreshed.Note != note || !refreshed.IsPriority {
		t.Fatalf("manual fields were overwritten by observation: %+v", refreshed)
	}
	if refreshed.ProfileSource != insight.AccountProfileSourceMixed {
		t.Fatalf("expected mixed profile source, got %q", refreshed.ProfileSource)
	}
}

func TestAccountServicePersistsAndRefreshesObservedProfileAttributes(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	service := insight.NewAccountService(insight.AccountServiceConfig{
		Repository: insightdemo.NewAccountRepository(),
		Now:        func() time.Time { return now },
	})
	ctx := context.Background()
	query := insight.AccountQuery{HubUserID: "hub-user"}

	account, err := service.UpsertObservedProfile(ctx, query, insight.AccountObservedProfile{
		ChannelUsername: "finder-profile",
		ChannelName:     "画像账号",
		Signature:       "账号简介",
		AvatarURL:       "https://example.com/avatar.png",
		Region:          "河南 郑州",
		Industry:        "新闻传媒",
		Tags:            []string{"媒体", "新闻"},
		ObservedAt:      now,
	})
	if err != nil {
		t.Fatalf("upsert observed profile: %v", err)
	}
	if account.Region != "河南 郑州" || account.Industry != "新闻传媒" || len(account.Tags) != 2 {
		t.Fatalf("observed profile attributes were not persisted: %+v", account)
	}

	refreshed, err := service.UpsertObservedProfile(ctx, query, insight.AccountObservedProfile{
		ChannelUsername: "finder-profile",
		ChannelName:     "更新后的账号",
		Region:          "北京",
		Industry:        "科技",
		Tags:            []string{"科技", "AI"},
		ObservedAt:      now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("refresh observed profile: %v", err)
	}
	if refreshed.Region != "北京" || refreshed.Industry != "科技" || len(refreshed.Tags) != 2 || refreshed.Tags[0] != "科技" {
		t.Fatalf("new upstream profile attributes were not refreshed: %+v", refreshed)
	}

	partial, err := service.UpsertObservedProfile(ctx, query, insight.AccountObservedProfile{
		ChannelUsername: "finder-profile",
		ObservedAt:      now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("refresh partial observed profile: %v", err)
	}
	if partial.ChannelName != "更新后的账号" || partial.Signature != "账号简介" || partial.AvatarURL != "https://example.com/avatar.png" || partial.Region != "北京" || partial.Industry != "科技" || len(partial.Tags) != 2 {
		t.Fatalf("partial upstream response should not erase profile attributes: %+v", partial)
	}
}

func TestAccountServiceFiltersAndPaginatesWorkspaceAccounts(t *testing.T) {
	repo := insightdemo.NewAccountRepository()
	service := insight.NewAccountService(insight.AccountServiceConfig{Repository: repo})
	ctx := context.Background()
	for _, username := range []string{"finder-a", "finder-b", "finder-c"} {
		if _, err := service.UpsertObservedProfile(ctx, insight.AccountQuery{HubUserID: "hub-user"}, insight.AccountObservedProfile{
			ChannelUsername: username,
			ChannelName:     username,
		}); err != nil {
			t.Fatalf("seed %s: %v", username, err)
		}
	}
	region := "华南"
	all, err := service.List(ctx, insight.AccountQuery{HubUserID: "hub-user", PageSize: 20, Region: region})
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if all.Total != 0 {
		t.Fatalf("expected no region match before manual update, got %d", all.Total)
	}
	account, err := service.List(ctx, insight.AccountQuery{HubUserID: "hub-user", PageSize: 20})
	if err != nil || account.Total != 3 {
		t.Fatalf("expected three accounts, got page=%+v err=%v", account, err)
	}
	if account.TotalPages != 1 || len(account.Items) != 3 {
		t.Fatalf("unexpected pagination: %+v", account)
	}
}

func TestAccountServiceSyncObservedProfilesDedupesAndPreservesManualProfile(t *testing.T) {
	service := insight.NewAccountService(insight.AccountServiceConfig{
		Repository: insightdemo.NewAccountRepository(),
		Now:        func() time.Time { return time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC) },
	})
	ctx := context.Background()
	query := insight.AccountQuery{HubUserID: "hub-user"}
	if _, err := service.UpsertObservedProfile(ctx, query, insight.AccountObservedProfile{
		ChannelUsername: "finder-sync",
		ChannelName:     "人工维护前昵称",
		ObservedAt:      time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, err := service.List(ctx, query)
	if err != nil || len(accounts.Items) != 1 {
		t.Fatalf("list seeded account: page=%+v err=%v", accounts, err)
	}
	region := "华东"
	if _, err := service.UpdateManualProfile(ctx, query, accounts.Items[0].ID, insight.AccountProfilePatch{Region: &region}); err != nil {
		t.Fatalf("set manual profile: %v", err)
	}

	count, err := service.SyncObservedProfiles(ctx, query, []insight.AccountObservedProfile{
		{ChannelUsername: "finder-sync", ChannelName: "旧昵称", ObservedAt: time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)},
		{ChannelUsername: "finder-sync", ChannelName: "最新昵称", ObservedAt: time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)},
		{ChannelUsername: "finder-new", ChannelName: "新增账号", ObservedAt: time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)},
	})
	if err != nil || count != 2 {
		t.Fatalf("sync observed profiles: count=%d err=%v", count, err)
	}
	page, err := service.List(ctx, query)
	if err != nil || page.Total != 2 {
		t.Fatalf("expected two synced accounts: page=%+v err=%v", page, err)
	}
	var synced insight.Account
	for _, item := range page.Items {
		if item.ChannelUsername == "finder-sync" {
			synced = item
		}
	}
	if synced.ChannelName != "最新昵称" || synced.Region != region {
		t.Fatalf("sync should keep latest observed and manual fields: %+v", synced)
	}
}
