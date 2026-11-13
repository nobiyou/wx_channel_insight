package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/edge"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

const channelAssistantLoginURL = "https://channels.weixin.qq.com/platform/login"

type channelAccountSession struct {
	Status         string         `json:"status"`
	ClientID       string         `json:"clientId,omitempty"`
	FinderUsername string         `json:"finderUsername,omitempty"`
	LoginQRCode    string         `json:"loginQRCode,omitempty"`
	LoginQRCodeURL string         `json:"loginQRCodeURL,omitempty"`
	Node           *edge.NodeInfo `json:"node,omitempty"`
	ActionRequired string         `json:"actionRequired,omitempty"`
}

func (s *server) handleChannelAccountSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.edgeHub == nil {
		writeJSON(w, http.StatusOK, channelAccountSession{Status: "offline", ActionRequired: "请先启动 wx_channel 并连接到 Insight"})
		return
	}
	node, ok := selectChannelAccountNode(s.edgeHub, r.URL.Query().Get("clientId"))
	if !ok {
		writeJSON(w, http.StatusOK, channelAccountSession{Status: "offline", ActionRequired: "请先启动 wx_channel 并连接到 Insight"})
		return
	}
	status := "login_required"
	action := "请在视频号助手完成登录"
	if strings.TrimSpace(node.FinderUsername) != "" {
		status = "authenticated"
		action = ""
	}
	writeJSON(w, http.StatusOK, channelAccountSession{
		Status:         status,
		ClientID:       node.ID,
		FinderUsername: node.FinderUsername,
		LoginQRCode:    node.LoginQRCode,
		LoginQRCodeURL: node.LoginQRCodeURL,
		Node:           &node,
		ActionRequired: action,
	})
}

func (s *server) handleChannelAccountLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.edgeHub == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "边缘连接未启用，请先启动 wx_channel"})
		return
	}
	var payload struct {
		ClientID string `json:"clientId"`
	}
	if r.Body != nil {
		if err := decodeJSONBody(r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	node, ok := selectChannelAccountNode(s.edgeHub, payload.ClientID)
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "没有可用的视频号助手连接"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	response, err := s.edgeHub.SendCommand(ctx, node.ID, "channel_navigate", map[string]string{"url": channelAssistantLoginURL}, 15*time.Second)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "打开视频号助手失败: " + err.Error()})
		return
	}
	if !response.Success {
		message := strings.TrimSpace(response.Error)
		if message == "" {
			message = "边缘页面拒绝了导航请求"
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": message})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "login_pending",
		"clientId": node.ID,
		"url":      channelAssistantLoginURL,
	})
}

func (s *server) handleChannelAccountCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.edgeHub == nil || s.ingestService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "视频号采集连接未配置"})
		return
	}
	var payload struct {
		ClientID string `json:"clientId"`
		Username string `json:"username"`
	}
	if r.Body != nil {
		if err := decodeJSONBody(r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	node, ok := selectChannelAccountNode(s.edgeHub, payload.ClientID)
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "没有可用的视频号助手连接"})
		return
	}
	username := strings.TrimSpace(payload.Username)
	if username == "" {
		username = strings.TrimSpace(node.FinderUsername)
	}
	if username == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "尚未检测到已登录的视频号，请先完成登录"})
		return
	}
	requestContext := hubContextFromRequest(r)
	result, summary, err := s.ingestService.ImportWXFeedListForAccount(r.Context(), ingest.SummaryQuery{
		HubUserID: requestContext.HubUserID, DeviceCount: requestContext.DeviceCount,
	}, username, node.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "本账号作品采集失败: " + err.Error()})
		return
	}
	var account *insight.Account
	if s.accountService != nil {
		profiles, profileErr := s.ingestService.SearchWXFeedAccounts(r.Context(), ingest.SummaryQuery{
			HubUserID: requestContext.HubUserID, DeviceCount: requestContext.DeviceCount,
		}, "", username, node.ID)
		if profileErr == nil && len(profiles) > 0 {
			profile := profiles[0]
			if profile.Username == "" {
				profile.Username = username
			}
			stored, saveErr := s.accountService.UpsertObservedProfile(r.Context(), insight.AccountQuery{
				HubUserID: requestContext.HubUserID, DeviceCount: requestContext.DeviceCount, ContextSource: "channel_login",
			}, insight.AccountObservedProfile{
				Platform: insight.AccountPlatformWXChannels, ChannelUsername: profile.Username,
				ChannelName: profile.Nickname, Signature: profile.Signature, AvatarURL: profile.HeadURL,
				Region: profile.Region, Industry: profile.Industry, Tags: append([]string(nil), profile.Tags...), ObservedAt: time.Now().UTC(),
			})
			if saveErr == nil {
				account = &stored
			}
		}
	}
	if refreshErr := s.refreshDashboardAfterIngest(r.Context(), requestContext); refreshErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": refreshErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "collected", "clientId": node.ID, "username": username,
		"itemCount": len(result.Items), "pageCount": result.PageCount, "complete": result.Complete,
		"nextMarker": result.NextMarker, "summary": summary, "account": account,
	})
}

func selectChannelAccountNode(hub *edge.Hub, clientID string) (edge.NodeInfo, bool) {
	if hub == nil {
		return edge.NodeInfo{}, false
	}
	requested := strings.TrimSpace(clientID)
	for _, node := range hub.List() {
		if node.Status == "stale" {
			continue
		}
		if requested == "" || node.ID == requested {
			return node, true
		}
	}
	return edge.NodeInfo{}, false
}

func decodeJSONBody(r *http.Request, target any) error {
	if r == nil || r.Body == nil {
		return nil
	}
	err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(target)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
