package controllers_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/config"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
)

var errDeliveryBoom = errors.New("delivery boom")

func newSessionStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{}, httpd.ControlDeps{}))
	t.Cleanup(srv.Close)
	return srv
}

// ---- merge-local ----

func TestSessionsRoutes_MergeLocal_200(t *testing.T) {
	svc := newFakeSessionService()
	srv := newSessionTestServer(t, svc)

	body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/sessions/open-agents-1/merge-local", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, body)
	}
	var resp struct {
		OK            bool   `json:"ok"`
		SessionID     string `json:"sessionId"`
		TargetBranch  string `json:"targetBranch"`
		TargetHeadSHA string `json:"targetHeadSha"`
		BranchRemoved bool   `json:"branchRemoved"`
	}
	mustJSON(t, body, &resp)
	if !resp.OK || resp.SessionID != "open-agents-1" || resp.TargetBranch != "dev" || resp.TargetHeadSHA == "" || !resp.BranchRemoved {
		t.Errorf("resp = %+v, want merged dev result", resp)
	}
}

func TestSessionsRoutes_MergeLocal_ErrorEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
		code string
	}{
		{"dirty checkout", apierr.Conflict("WORKSPACE_DIRTY", "dirty", nil), http.StatusConflict, "WORKSPACE_DIRTY"},
		{"not on dev", apierr.Conflict("CHECKOUT_NOT_ON_DEV", "not on dev", nil), http.StatusConflict, "CHECKOUT_NOT_ON_DEV"},
		{"conflict", apierr.Conflict("LOCAL_MERGE_CONFLICT", "conflict", nil), http.StatusConflict, "LOCAL_MERGE_CONFLICT"},
		{"unknown session", apierr.NotFound("SESSION_NOT_FOUND", "unknown"), http.StatusNotFound, "SESSION_NOT_FOUND"},
		{"unconfigured", apierr.NotImplemented("DELIVERY_UNAVAILABLE", "unconfigured"), http.StatusNotImplemented, "DELIVERY_UNAVAILABLE"},
		{"internal", errDeliveryBoom, http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeSessionService()
			svc.mergeLocalErr = tc.err
			srv := newSessionTestServer(t, svc)
			body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/sessions/open-agents-1/merge-local", "")
			assertErrorCode(t, body, status, tc.want, tc.code)
		})
	}
}

func TestSessionsRoutes_MergeLocal_NilService(t *testing.T) {
	srv := newSessionStubServer(t)
	body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/sessions/open-agents-1/merge-local", "")
	assertErrorCode(t, body, status, http.StatusNotImplemented, "NOT_IMPLEMENTED")
}

// ---- create PR ----

func TestSessionsRoutes_CreatePR_200(t *testing.T) {
	svc := newFakeSessionService()
	srv := newSessionTestServer(t, svc)

	body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/sessions/open-agents-1/pr", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, body)
	}
	var resp struct {
		OK       bool   `json:"ok"`
		PRURL    string `json:"prUrl"`
		PRNumber int    `json:"prNumber"`
		Created  bool   `json:"created"`
	}
	mustJSON(t, body, &resp)
	if !resp.OK || resp.PRURL != "https://github.com/acme/repo/pull/8" || resp.PRNumber != 8 || !resp.Created {
		t.Errorf("resp = %+v, want created PR 8", resp)
	}
}

func TestSessionsRoutes_CreatePR_ErrorEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
		code string
	}{
		{"auth missing", apierr.Forbidden("GH_AUTH_MISSING", "auth"), http.StatusForbidden, "GH_AUTH_MISSING"},
		{"push rejected", apierr.Conflict("PUSH_REJECTED", "rejected", nil), http.StatusConflict, "PUSH_REJECTED"},
		{"unknown session", apierr.NotFound("SESSION_NOT_FOUND", "unknown"), http.StatusNotFound, "SESSION_NOT_FOUND"},
		{"unconfigured", apierr.NotImplemented("DELIVERY_UNAVAILABLE", "unconfigured"), http.StatusNotImplemented, "DELIVERY_UNAVAILABLE"},
		{"internal", errDeliveryBoom, http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeSessionService()
			svc.createPRErr = tc.err
			srv := newSessionTestServer(t, svc)
			body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/sessions/open-agents-1/pr", "")
			assertErrorCode(t, body, status, tc.want, tc.code)
		})
	}
}

func TestSessionsRoutes_CreatePR_NilService(t *testing.T) {
	srv := newSessionStubServer(t)
	body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/sessions/open-agents-1/pr", "")
	assertErrorCode(t, body, status, http.StatusNotImplemented, "NOT_IMPLEMENTED")
}
