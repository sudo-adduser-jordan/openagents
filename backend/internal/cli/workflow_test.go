package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// workflowModeCapture records what the CLI PATCHed to the daemon.
type workflowModeCapture struct {
	path string
	body map[string]string
}

// workflowModeServer answers GET sessions/{id} (project scoping) and
// PATCH sessions/{id}/workflow-mode, capturing the PATCH request. Errors and
// not-found are served as close to the daemon as the test needs: pass a handler
// for full control.
func workflowModeServer(t *testing.T, patch func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *workflowModeCapture) {
	t.Helper()
	capture := &workflowModeCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sessions/demo-1":
			_, _ = io.WriteString(w, `{"session":{"id":"demo-1","projectId":"demo","kind":"worker","activity":{"state":"idle"},"prs":[]}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/sessions/demo-1/workflow-mode":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read patch body: %v", err)
			}
			capture.path = r.URL.Path
			capture.body = map[string]string{}
			if err := json.Unmarshal(body, &capture.body); err != nil {
				t.Fatalf("decode patch body %q: %v", body, err)
			}
			patch(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, capture
}

func TestPlan_Success(t *testing.T) {
	cfg := setConfigEnv(t)
	srv, capture := workflowModeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"sessionId":"demo-1","workflowMode":"planning","session":{"id":"demo-1","projectId":"demo","kind":"worker"}}`)
	})
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "plan", "demo-1")
	if err != nil {
		t.Fatalf("ao plan failed: %v\nstderr=%s", err, errOut)
	}
	if capture.path != "/api/v1/sessions/demo-1/workflow-mode" {
		t.Fatalf("path = %q, want the workflow-mode route", capture.path)
	}
	if capture.body["workflowMode"] != "planning" {
		t.Fatalf("request body = %#v, want workflowMode planning", capture.body)
	}
	if !strings.Contains(out, "session demo-1 set to planning") {
		t.Fatalf("unexpected plan output:\n%s", out)
	}
}

func TestBuild_Success(t *testing.T) {
	cfg := setConfigEnv(t)
	srv, capture := workflowModeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"sessionId":"demo-1","workflowMode":"building","session":{"id":"demo-1","projectId":"demo","kind":"worker"}}`)
	})
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "build", "demo-1")
	if err != nil {
		t.Fatalf("ao build failed: %v\nstderr=%s", err, errOut)
	}
	if capture.body["workflowMode"] != "building" {
		t.Fatalf("request body = %#v, want workflowMode building", capture.body)
	}
	if !strings.Contains(out, "session demo-1 set to building") {
		t.Fatalf("unexpected build output:\n%s", out)
	}
}

func TestPlan_ProjectScopeVerifiesSessionFirst(t *testing.T) {
	cfg := setConfigEnv(t)
	srv, capture := workflowModeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"sessionId":"demo-1","workflowMode":"planning","session":{"id":"demo-1","projectId":"demo","kind":"worker"}}`)
	})
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "plan", "demo-1", "-p", "demo")
	if err != nil {
		t.Fatalf("ao plan with project scope failed: %v\nstderr=%s", err, errOut)
	}
	if capture.body["workflowMode"] != "planning" {
		t.Fatalf("request body = %#v, want workflowMode planning", capture.body)
	}
	if !strings.Contains(out, "session demo-1 set to planning") {
		t.Fatalf("unexpected plan output:\n%s", out)
	}
}

func TestPlan_MissingIDIsUsageError(t *testing.T) {
	setConfigEnv(t)
	_, _, err := executeCLI(t, Deps{}, "plan")
	if err == nil {
		t.Fatal("expected missing id to fail")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", got, err)
	}
}

func TestBuild_MissingIDIsUsageError(t *testing.T) {
	setConfigEnv(t)
	_, _, err := executeCLI(t, Deps{}, "build")
	if err == nil {
		t.Fatal("expected missing id to fail")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", got, err)
	}
}

func TestPlan_SurfacesDaemonErrorEnvelope(t *testing.T) {
	cfg := setConfigEnv(t)
	srv, _ := workflowModeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not_found","code":"SESSION_NOT_FOUND","message":"Unknown session","requestId":"req-plan-1"}`)
	})
	writeRunFileFor(t, cfg, srv)

	_, _, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "plan", "demo-1")
	if err == nil {
		t.Fatal("expected the daemon error to surface")
	}
	if got := ExitCode(err); got != 1 {
		t.Fatalf("exit code = %d, want 1 (err=%v)", got, err)
	}
	for _, want := range []string{"Unknown session", "SESSION_NOT_FOUND", "req-plan-1"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to contain %q", err, want)
		}
	}
}
