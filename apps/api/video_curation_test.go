package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestVideoCurationAPIChangesLibraryAndRetainsFacts(t *testing.T) {
	store, err := insightfile.NewVideoCurationRepository("")
	if err != nil {
		t.Fatal(err)
	}
	srv := newDataCenterTestServer()
	srv.dataCenterService = insight.NewDataCenterService(insight.DataCenterServiceConfig{CurationRepository: store, DataProvider: apiDataCenterProvider{events: []ingest.Event{{ID: "e", ObjectID: "real", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: time.Now(), LikeCount: 20}}}})
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRecorder()
		srv.routes().ServeHTTP(r, httptest.NewRequest(method, dataCenterRoutePrefix+path, bytes.NewBufferString(body)))
		return r
	}
	for _, body := range []string{`{"ids":["video:real"],"status":"deleted"}`, `{"ids":["video:real"],"note":5}`, `{"ids":[],"status":"selected"}`, `{"ids":["video:real"],"unexpected":true}`, `{"ids":["video:real"],"status":"selected"} {}`} {
		r := request(http.MethodPost, "videos/curation", body)
		if r.Code != http.StatusBadRequest {
			t.Fatalf("invalid request saved: %d %s", r.Code, r.Body.String())
		}
	}
	r := request(http.MethodPost, "videos/curation", `{"ids":["video:real","video:unknown"],"status":"selected"}`)
	if r.Code != http.StatusNotFound {
		t.Fatalf("unknown video accepted: %d %s", r.Code, r.Body.String())
	}
	r = request(http.MethodPost, "videos/curation", `{"ids":["video:real"],"status":"selected","tags":["开场参考"],"note":"保留原始事实"}`)
	if r.Code != http.StatusOK {
		t.Fatalf("save failed: %d %s", r.Code, r.Body.String())
	}
	r = request(http.MethodGet, "videos?curation=selected&q=保留原始事实", "")
	var page struct {
		Total int
		Items []struct {
			ObjectID string                `json:"objectId"`
			Likes    int                   `json:"likes"`
			Curation insight.VideoCuration `json:"curation"`
		}
	}
	if err = json.Unmarshal(r.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusOK || page.Total != 1 || page.Items[0].Likes != 20 || page.Items[0].Curation.Status != "selected" {
		t.Fatalf("selection filter or facts lost: %s", r.Body.String())
	}
	r = request(http.MethodGet, "videos/video%3Areal/curation", "")
	var choice insight.VideoCuration
	if err = json.Unmarshal(r.Body.Bytes(), &choice); err != nil || r.Code != http.StatusOK || choice.Tags[0] != "开场参考" {
		t.Fatalf("detail decision missing: %s %v", r.Body.String(), err)
	}
	for _, filter := range []string{"minLikes=-1", "minInteractions=bad", "freshOnly=maybe", "curation=deleted", "performance=bad"} {
		r = request(http.MethodGet, "videos?"+filter, "")
		if r.Code != http.StatusBadRequest {
			t.Fatalf("invalid filter %s accepted: %d", filter, r.Code)
		}
	}
}
