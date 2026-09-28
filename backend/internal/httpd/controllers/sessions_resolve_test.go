package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/controllers"
)

func resolveRefRequest(t *testing.T, svc controllers.SessionService, query string) (int, []byte) {
	t.Helper()
	r := chi.NewRouter()
	(&controllers.SessionsController{Svc: svc}).Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sessions/resolve?"+query, nil))
	return rec.Code, rec.Body.Bytes()
}

func TestResolveSessionRefRouteReturnsTheCanonicalTarget(t *testing.T) {
	svc := newFakeSessionService()
	svc.sessions["openagents-5"] = domain.Session{SessionRecord: domain.SessionRecord{ID: "openagents-5", ProjectID: "openagents"}}
	svc.nums["openagents-5"] = 5

	status, body := resolveRefRequest(t, svc, "ref=5&project=openagents")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", status, body)
	}
	var got controllers.ResolveSessionRefResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if got.SessionID != "openagents-5" || got.ProjectID != "openagents" || got.Num != 5 {
		t.Errorf("response = %+v, want the resolved session, project, and number", got)
	}
	if got.Ref != "5" {
		t.Errorf("Ref = %q, want the requested reference echoed", got.Ref)
	}
	if got.MatchedBy != "num" {
		t.Errorf("MatchedBy = %q, want %q", got.MatchedBy, "num")
	}
}

func TestResolveSessionRefRouteReportsTheOwningProjectWhenUnscoped(t *testing.T) {
	svc := newFakeSessionService()
	svc.sessions["openagents-5"] = domain.Session{SessionRecord: domain.SessionRecord{ID: "openagents-5", ProjectID: "openagents"}}
	svc.nums["openagents-5"] = 5

	// An unscoped number still resolves, and the response tells the client which
	// project owns it — otherwise the caller cannot scope its next request.
	_, body := resolveRefRequest(t, svc, "ref=5")
	var got controllers.ResolveSessionRefResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if got.ProjectID != "openagents" {
		t.Errorf("ProjectID = %q, want the owning project openagents", got.ProjectID)
	}
}

func TestResolveSessionRefRouteOmitsProjectForProjectlessSession(t *testing.T) {
	svc := newFakeSessionService()
	svc.sessions["bare-9"] = domain.Session{SessionRecord: domain.SessionRecord{ID: "bare-9"}}
	svc.nums["bare-9"] = 9

	// A projectless session has no project to report, and an empty string is
	// dropped from the wire rather than sent as a null scope.
	_, body := resolveRefRequest(t, svc, "ref=9")
	var got controllers.ResolveSessionRefResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if got.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty", got.ProjectID)
	}
	if containsAll(body, `"projectId"`) {
		t.Errorf("body = %s, want projectId omitted for a projectless session", body)
	}
}

func TestResolveSessionRefRoutePassesTheTrimmedRef(t *testing.T) {
	svc := newFakeSessionService()
	// A padded ref must reach the service trimmed, or it would look up a
	// reference the user never typed.
	_, body := resolveRefRequest(t, svc, "ref="+url.QueryEscape("  openagents-1  "))
	if !containsAll(body, "openagents-1") {
		t.Errorf("body = %s, want the trimmed ref resolved", body)
	}
}

func TestResolveSessionRefRouteWritesTheErrorEnvelope(t *testing.T) {
	svc := newFakeSessionService()
	svc.resolveRefErr = apierr.Conflict("SESSION_REF_AMBIGUOUS", "session number 5 matches more than one session", nil)

	status, body := resolveRefRequest(t, svc, "ref=5")
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", status, body)
	}
	// The code is the only thing the CLI can branch on, so it has to survive to
	// the wire.
	if !containsAll(body, "SESSION_REF_AMBIGUOUS", "matches more than one session") {
		t.Errorf("body = %s, want the resolver's envelope", body)
	}
}

func TestResolveSessionRefRouteWithoutServiceIsNotImplemented(t *testing.T) {
	r := chi.NewRouter()
	(&controllers.SessionsController{}).Register(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sessions/resolve?ref=5", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 (%s)", rec.Code, rec.Body)
	}
}
