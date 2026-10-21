package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/runtime"
)

func TestRetiredContentWorkflowsCannotExecute(t *testing.T) {
	srv := newServer(nil, nil, nil, nil, runtime.Status{})
	for _, path := range []string{
		"/api/demo/dashboard",
		"/api/v1/production/render",
		"/api/v1/production/assets/reference",
		"/api/v1/production/tasks/old-task/render",
		"/api/v1/production/outputs/old-task/version.mp4",
		"/api/v1/dashboard/team-follow-ups",
		"/api/v1/dashboard/team-follow-ups/analyze",
		"/api/v1/read-models/topic-recommendations",
		"/api/v1/read-models/today-topic-recommendations",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(method+" "+path, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				srv.routes().ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(`{}`)))
				if recorder.Code != http.StatusNotFound {
					t.Fatalf("removed workflow must have no registered route: status=%d body=%s", recorder.Code, recorder.Body.String())
				}
			})
		}
	}
}
