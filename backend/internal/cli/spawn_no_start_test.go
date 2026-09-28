package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// noStartServer serves the request sequence a `spawn --no-start` run performs
// and captures the spawn body so tests can assert what actually went over the
// wire.
func noStartServer(t *testing.T, requests *[]string, got *spawnRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appendPrimaryRequest(requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/demo":
			_, _ = io.WriteString(w, `{"status":"ok","project":{"id":"demo","name":"Demo","path":"/repo/demo","repo":"https://github.com/sudo-adduser-jordan/open-agents","defaultBranch":"main"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/readiness/ensure":
			_, _ = io.WriteString(w, authorizedAgentsJSON("opencode"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions":
			_ = json.NewDecoder(r.Body).Decode(got)
			_, _ = io.WriteString(w, `{"session":{"id":"demo-9","status":"pending"},"promptBytes":42}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// `spawn --no-start` must create the session through the normal spawn route with
// the flag set, not through some second endpoint, so everything that makes a task
// real (project, branch, worktree, resolved prompt) still happens.
func TestSpawnNoStartSendsNoStartAndReportsPending(t *testing.T) {
	cfg := setConfigEnv(t)
	var requests []string
	var got spawnRequest
	srv := noStartServer(t, &requests, &got)
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }},
		"spawn", "--project", "demo", "--agent", "opencode", "--name", "worker", "--prompt", "do the thing", "--no-start")
	if err != nil {
		t.Fatalf("spawn --no-start failed: %v stderr=%s", err, errOut)
	}

	if !got.NoStart {
		t.Errorf("spawn request NoStart=false, want true")
	}
	if got.Prompt != "do the thing" {
		t.Errorf("spawn request prompt = %q, want %q", got.Prompt, "do the thing")
	}

	// The session id and status have to reach the operator, and the fact that
	// nothing is running has to be stated rather than left to be inferred.
	if !strings.Contains(out, "demo-9") {
		t.Errorf("output missing session id: %s", out)
	}
	if !strings.Contains(out, "pending") {
		t.Errorf("output missing pending status: %s", out)
	}
	if !strings.Contains(out, "agent not started") {
		t.Errorf("output does not say the agent was not started: %s", out)
	}
	if !strings.Contains(out, "open-agents session resume-agent demo-9") {
		t.Errorf("output does not say how to start it later: %s", out)
	}

	// Readiness still runs: a staged task is meant to be startable later, so
	// surfacing an unusable agent now is more useful than deferring it.
	want := []string{"GET /api/v1/projects/demo", "POST /api/v1/agents/readiness/ensure", "POST /api/v1/sessions"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests=%#v want %#v", requests, want)
	}
}

// Without the flag the wire body must be unchanged: `noStart` is omitempty, so
// ordinary spawns are byte-identical to before.
func TestSpawnWithoutNoStartOmitsTheField(t *testing.T) {
	cfg := setConfigEnv(t)
	var requests []string
	var got spawnRequest
	srv := noStartServer(t, &requests, &got)
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }},
		"spawn", "--project", "demo", "--agent", "opencode", "--name", "worker")
	if err != nil {
		t.Fatalf("spawn failed: %v stderr=%s", err, errOut)
	}
	if got.NoStart {
		t.Errorf("spawn request NoStart=true, want false")
	}
	if strings.Contains(out, "agent not started") {
		t.Errorf("ordinary spawn reported a staged agent: %s", out)
	}
}

// --skip-agent-check is the documented bypass for the readiness preflight, and
// it has to keep working for a staged spawn rather than only a running one.
func TestSpawnNoStartWithSkipAgentCheckSkipsReadiness(t *testing.T) {
	cfg := setConfigEnv(t)
	var requests []string
	var got spawnRequest
	srv := noStartServer(t, &requests, &got)
	writeRunFileFor(t, cfg, srv)

	_, errOut, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }},
		"spawn", "--project", "demo", "--agent", "opencode", "--name", "worker", "--no-start", "--skip-agent-check")
	if err != nil {
		t.Fatalf("spawn --no-start --skip-agent-check failed: %v stderr=%s", err, errOut)
	}
	if !got.NoStart {
		t.Errorf("spawn request NoStart=false, want true")
	}
	want := []string{"GET /api/v1/projects/demo", "POST /api/v1/sessions"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests=%#v want %#v", requests, want)
	}
}

// Claiming a PR is about ownership of a review, not about a running process, so
// it must still work on a staged session -- that is the point of creating tasks
// ahead of time.
func TestSpawnNoStartWithClaimPR(t *testing.T) {
	cfg := setConfigEnv(t)
	var requests []string
	var got spawnRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appendPrimaryRequest(&requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/demo":
			_, _ = io.WriteString(w, `{"status":"ok","project":{"id":"demo","name":"Demo","path":"/repo/demo","repo":"https://github.com/sudo-adduser-jordan/open-agents","defaultBranch":"main"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/readiness/ensure":
			_, _ = io.WriteString(w, authorizedAgentsJSON("opencode"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions":
			_ = json.NewDecoder(r.Body).Decode(&got)
			_, _ = io.WriteString(w, `{"session":{"id":"demo-9","status":"pending"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions/demo-9/pr/claim":
			_, _ = io.WriteString(w, `{"ok":true,"sessionId":"demo-9","prs":[{"url":"https://github.com/sudo-adduser-jordan/open-agents/pull/142","number":142,"state":"open","ci":"passing","review":"review_required","mergeability":"mergeable","reviewComments":false,"updatedAt":"2026-06-04T12:00:00Z"}],"branchChanged":false,"takenOverFrom":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }},
		"spawn", "--project", "demo", "--agent", "opencode", "--name", "worker", "--no-start", "--claim-pr", "142", "--no-takeover")
	if err != nil {
		t.Fatalf("spawn --no-start --claim-pr failed: %v stderr=%s", err, errOut)
	}
	if !got.NoStart {
		t.Errorf("spawn request NoStart=false, want true")
	}
	if !strings.Contains(out, "claimed https://github.com/sudo-adduser-jordan/open-agents/pull/142") {
		t.Errorf("output missing claimed label: %s", out)
	}
	if !strings.Contains(out, "agent not started") {
		t.Errorf("output does not say the agent was not started: %s", out)
	}
	want := []string{"GET /api/v1/projects/demo", "POST /api/v1/agents/readiness/ensure", "POST /api/v1/sessions", "POST /api/v1/sessions/demo-9/pr/claim"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests=%#v want %#v", requests, want)
	}
}

// A daemon error envelope has to surface unchanged, so an operator staging a
// task sees the daemon's reason rather than a CLI paraphrase.
func TestSpawnNoStartSurfacesDaemonErrorEnvelope(t *testing.T) {
	cfg := setConfigEnv(t)
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appendPrimaryRequest(&requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/demo":
			_, _ = io.WriteString(w, `{"status":"ok","project":{"id":"demo","name":"Demo","path":"/repo/demo","repo":"https://github.com/sudo-adduser-jordan/open-agents","defaultBranch":"main"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/readiness/ensure":
			_, _ = io.WriteString(w, authorizedAgentsJSON("opencode"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions":
			w.Header().Set("X-Request-Id", "req-42")
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":"conflict","code":"WORKSPACE_CREATE_FAILED","message":"branch already has a worktree","requestId":"req-42"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	writeRunFileFor(t, cfg, srv)

	_, _, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }},
		"spawn", "--project", "demo", "--agent", "opencode", "--name", "worker", "--no-start")
	if err == nil {
		t.Fatal("spawn --no-start succeeded, want the daemon error")
	}
	if !strings.Contains(err.Error(), "WORKSPACE_CREATE_FAILED") {
		t.Errorf("error = %v, want the daemon error code", err)
	}
	if !strings.Contains(err.Error(), "branch already has a worktree") {
		t.Errorf("error = %v, want the daemon message", err)
	}
	if !strings.Contains(err.Error(), "req-42") {
		t.Errorf("error = %v, want the request id preserved", err)
	}
}
