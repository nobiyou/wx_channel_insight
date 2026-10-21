package main

import (
	"context"
	"encoding/json"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"net/http"
	"net/http/httptest"
	"testing"
)

type incompleteAccountRepository struct {
	insight.AccountRepository
	account insight.Account
}

func (r incompleteAccountRepository) Get(context.Context, string, string) (insight.Account, bool, error) {
	return r.account, true, nil
}

func TestAccountVideosRejectIncompleteIdentity(t *testing.T) {
	srv, account := newAccountAPITestServer(t)
	account.ChannelUsername = ""
	account.IdentityStatus = insight.AccountIdentityIncomplete
	srv.accountService = insight.NewAccountService(insight.AccountServiceConfig{Repository: incompleteAccountRepository{account: account}, WorkspaceContextResolver: fixedAccountContextResolver{workspaceID: "workspace:accounts"}})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+account.ID+"/videos?hub_user_id=hub:user:accounts", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("incomplete account must not reveal all workspace videos: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccountVideosApplyAndValidateObservationDates(t *testing.T) {
	srv, account := newAccountAPITestServer(t)
	for _, test := range []struct {
		query  string
		status int
		total  int
	}{
		{"from=2026-08-17", http.StatusOK, 0},
		{"from=invalid", http.StatusBadRequest, 0},
		{"from=2026-08-15&to=2026-08-17", http.StatusOK, 1},
	} {
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+account.ID+"/videos?hub_user_id=hub:user:accounts&"+test.query, nil))
		if rec.Code != test.status {
			t.Fatalf("%s: %d %s", test.query, rec.Code, rec.Body.String())
		}
		if test.status == http.StatusOK {
			var p struct{ Total int }
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if p.Total != test.total {
				t.Fatalf("%s total %d, want %d", test.query, p.Total, test.total)
			}
		}
	}
}
