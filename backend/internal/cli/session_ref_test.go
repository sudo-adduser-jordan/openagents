package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// resolveRoutes is a daemon stand-in that answers the resolver, the session read
// used to learn a sender's project, and the send itself, so a test can assert the
// whole addressing path rather than one call in isolation.
type resolveRoutes struct {
	// resolve is keyed by "ref" or "ref|project" and returns the status and body
	// for GET /api/v1/sessions/resolve. A miss is a 404 envelope.
	resolve map[string]resolveReply
	// sessions is keyed by session id for GET /api/v1/sessions/{id}.
	sessions map[string]string
	// sentPath and sentBody capture the final send.
	sentPath string
	sentBody string
	// steerReply overrides the steer-or-send response so a test can provoke the
	// recovery hint. Zero value answers {"ok":true}.
	steerReply resolveReply
	// resolveCalls records the resolver query strings in arrival order.
	resolveCalls []string
}

type resolveReply struct {
	status int
	body   string
}

func newResolveRoutes() *resolveRoutes {
	return &resolveRoutes{
		resolve:  map[string]resolveReply{},
		sessions: map[string]string{},
	}
}

func (rr *resolveRoutes) addResolve(ref, project string, status int, body string) {
	rr.resolve[ref+"|"+project] = resolveReply{status: status, body: body}
}

func (rr *resolveRoutes) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sessions/resolve":
			q := r.URL.Query()
			rr.resolveCalls = append(rr.resolveCalls, q.Encode())
			reply, ok := rr.resolve[q.Get("ref")+"|"+q.Get("project")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"error":"not_found","code":"SESSION_REF_NOT_FOUND","message":"no session"}`)
				return
			}
			w.WriteHeader(reply.status)
			_, _ = io.WriteString(w, reply.body)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/sessions/"):
			id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/"))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			body, ok := rr.sessions[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"error":"not_found","code":"SESSION_NOT_FOUND","message":"Unknown session"}`)
				return
			}
			_, _ = io.WriteString(w, body)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/sessions/"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read send body: %v", err)
				return
			}
			rr.sentPath = r.URL.Path
			rr.sentBody = string(body)
			if rr.steerReply.body == "" {
				_, _ = io.WriteString(w, `{"ok":true}`)
				return
			}
			w.WriteHeader(rr.steerReply.status)
			_, _ = io.WriteString(w, rr.steerReply.body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (rr *resolveRoutes) sentMessage(t *testing.T) string {
	t.Helper()
	var req struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(rr.sentBody), &req); err != nil {
		t.Fatalf("decode send body: %v\nbody=%s", err, rr.sentBody)
	}
	return req.Message
}

func TestSendResolvesBareAgentNumberBeforeSending(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	rr.addResolve("5", "", http.StatusOK,
		`{"ref":"5","sessionId":"openagents-5","projectId":"openagents","num":5,"matchedBy":"num"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "5", "--message", "hi"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	if rr.sentPath != "/api/v1/sessions/openagents-5/send" {
		t.Errorf("send path = %q, want /api/v1/sessions/openagents-5/send", rr.sentPath)
	}
	if got := rr.sentMessage(t); got != "hi" {
		t.Errorf("message = %q, want %q", got, "hi")
	}
}

func TestSendProjectFlagScopesAgentNumberLookup(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	rr.addResolve("5", "demo", http.StatusOK,
		`{"ref":"5","sessionId":"demo-5","projectId":"demo","num":5,"matchedBy":"num"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "5", "--project", "demo", "--message", "hi"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	if len(rr.resolveCalls) != 1 || !strings.Contains(rr.resolveCalls[0], "project=demo") {
		t.Fatalf("resolve calls = %v, want one call carrying project=demo", rr.resolveCalls)
	}
	if rr.sentPath != "/api/v1/sessions/demo-5/send" {
		t.Errorf("send path = %q, want /api/v1/sessions/demo-5/send", rr.sentPath)
	}
}

func TestSendAmbiguousAgentNumberIsUsageError(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	rr.addResolve("5", "", http.StatusConflict,
		`{"error":"conflict","code":"SESSION_REF_AMBIGUOUS","message":"session number 5 matches more than one session: openagents-5 (project openagents), acme-5 (project acme); scope the request to a single project to choose one","requestId":"req-7"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	_, _, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "5", "--message", "hi")
	if err == nil {
		t.Fatal("expected an error for an ambiguous number")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("exit code = %d, want 2 (usage)", got)
	}
	// The candidate list and request id are the only way the user can fix this,
	// so they must survive to the terminal.
	if !strings.Contains(err.Error(), "openagents-5") || !strings.Contains(err.Error(), "acme-5") {
		t.Errorf("error lost the candidate list: %v", err)
	}
	if !strings.Contains(err.Error(), "req-7") {
		t.Errorf("error lost the request id: %v", err)
	}
	if rr.sentPath != "" {
		t.Errorf("ambiguous number must not send, but posted to %q", rr.sentPath)
	}
}

func TestSendRetiredNumberSurfacesDistinctCodeAndExits1(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	rr.addResolve("4", "", http.StatusNotFound,
		`{"error":"not_found","code":"SESSION_NUM_RETIRED","message":"session number 4 was retired; retired numbers are never reused (retired in project openagents)","requestId":"req-9"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	_, _, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "4", "--message", "hi")
	if err == nil {
		t.Fatal("expected an error for a retired number")
	}
	if got := ExitCode(err); got != 1 {
		t.Fatalf("exit code = %d, want 1 (runtime)", got)
	}
	// A retired number must not be downgraded to a bare "not found": the gap is
	// permanent and tells the user their number will never come back.
	if !strings.Contains(err.Error(), "SESSION_NUM_RETIRED") {
		t.Errorf("error did not surface the retired code: %v", err)
	}
	if !strings.Contains(err.Error(), "never reused") {
		t.Errorf("error did not explain the permanent gap: %v", err)
	}
}

func TestSendUnknownNumberIsNotSwallowedByOldDaemonFallback(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	// A 404 that came from the resolver, not from a missing route. The literal
	// "7" must not be sent: the daemon already said it is not a session.
	rr.addResolve("7", "", http.StatusNotFound,
		`{"error":"not_found","code":"SESSION_REF_NOT_FOUND","message":"no session with id or number 7","requestId":"req-3"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	_, _, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "7", "--message", "hi")
	if err == nil {
		t.Fatal("expected an error for an unknown number")
	}
	if !strings.Contains(err.Error(), "SESSION_REF_NOT_FOUND") {
		t.Errorf("error did not surface the resolver's answer: %v", err)
	}
	if rr.sentPath != "" {
		t.Errorf("unresolved number must not send, but posted to %q", rr.sentPath)
	}
}

func TestSendFallsBackToLiteralTargetOnDaemonWithoutResolver(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	// An older daemon has no /sessions/resolve and answers the unknown route.
	rr := newResolveRoutes()
	rr.addResolve("5", "", http.StatusNotFound,
		`{"error":"not_found","code":"ROUTE_NOT_FOUND","message":"no route for GET /api/v1/sessions/resolve"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "5", "--message", "hi"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	// Pre-numbering behavior preserved: the ref is passed through verbatim and
	// the daemon's own "no such session" error is what the user sees.
	if rr.sentPath != "/api/v1/sessions/5/send" {
		t.Errorf("send path = %q, want the literal /api/v1/sessions/5/send", rr.sentPath)
	}
}

func TestSendNonNumericSessionSkipsResolver(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "openagents-6", "--message", "hi"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	if len(rr.resolveCalls) != 0 {
		t.Errorf("resolver called for a nonnumeric target: %v", rr.resolveCalls)
	}
	if rr.sentPath != "/api/v1/sessions/openagents-6/send" {
		t.Errorf("send path = %q, want /api/v1/sessions/openagents-6/send", rr.sentPath)
	}
}

func TestSendPrefixesSenderAgentNumberWhenKnown(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "openagents-6")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	// The sender's own number, learned through the resolver.
	rr.addResolve("openagents-6", "", http.StatusOK,
		`{"ref":"openagents-6","sessionId":"openagents-6","projectId":"openagents","num":6,"matchedBy":"id"}`)
	rr.addResolve("9", "", http.StatusOK,
		`{"ref":"9","sessionId":"openagents-9","projectId":"openagents","num":9,"matchedBy":"num"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "9", "--message", "status?"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	// The recipient can now type "[from 6]" back; an id could not be shortened.
	if got, want := rr.sentMessage(t), "[from 6] status?"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestSendFallsBackToSenderIDWhenNumberUnknown(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "openagents-6")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	// A daemon that cannot resolve the sender: num stays unknown and the prefix
	// keeps the id rather than inventing a number.
	rr.addResolve("openagents-6", "", http.StatusNotFound,
		`{"error":"not_found","code":"ROUTE_NOT_FOUND","message":"no route for GET /api/v1/sessions/resolve"}`)
	rr.addResolve("9", "", http.StatusOK,
		`{"ref":"9","sessionId":"openagents-9","projectId":"openagents","num":9,"matchedBy":"num"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "9", "--message", "status?"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	if got, want := rr.sentMessage(t), "[from openagents-6] status?"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestSendSenderProjectScopesTargetWhenNoFlagGiven(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "openagents-6")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	rr.sessions["openagents-6"] = `{"session":{"id":"openagents-6","projectId":"acme"}}`
	rr.addResolve("5", "acme", http.StatusOK,
		`{"ref":"5","sessionId":"acme-5","projectId":"acme","num":5,"matchedBy":"num"}`)
	rr.addResolve("openagents-6", "", http.StatusOK,
		`{"ref":"openagents-6","sessionId":"openagents-6","projectId":"acme","num":6,"matchedBy":"id"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "5", "--message", "hi"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	if rr.sentPath != "/api/v1/sessions/acme-5/send" {
		t.Errorf("send path = %q, want /api/v1/sessions/acme-5/send", rr.sentPath)
	}
}

func TestSendProjectEnvScopesTargetWhenNoFlagGiven(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "acme")
	rr := newResolveRoutes()
	rr.addResolve("5", "acme", http.StatusOK,
		`{"ref":"5","sessionId":"acme-5","projectId":"acme","num":5,"matchedBy":"num"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "5", "--message", "hi"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	if rr.sentPath != "/api/v1/sessions/acme-5/send" {
		t.Errorf("send path = %q, want /api/v1/sessions/acme-5/send", rr.sentPath)
	}
}

func TestSendProjectFlagBeatsProjectEnv(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "acme")
	rr := newResolveRoutes()
	rr.addResolve("5", "demo", http.StatusOK,
		`{"ref":"5","sessionId":"demo-5","projectId":"demo","num":5,"matchedBy":"num"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "5", "--project", "demo", "--message", "hi"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	if rr.sentPath != "/api/v1/sessions/demo-5/send" {
		t.Errorf("send path = %q, want the flag's project, not the env's", rr.sentPath)
	}
}

func TestSendRecoverOnlyResolvesTargetWithoutSenderLookup(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "openagents-6")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	rr.addResolve("9", "", http.StatusOK,
		`{"ref":"9","sessionId":"openagents-9","projectId":"openagents","num":9,"matchedBy":"num"}`)
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	if _, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "9", "--steer", "--recover-only",
		"--client-message-id", "handle-1"); err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	// Recovery must stay a pure local lookup: the sender's number is only a
	// display nicety, so it must not add a request.
	if len(rr.resolveCalls) != 1 {
		t.Errorf("resolve calls = %v, want only the target resolution", rr.resolveCalls)
	}
}

func TestSendRecoverOnlyHintUsesResolvedSessionID(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("OPEN_AGENTS_SESSION_ID", "")
	t.Setenv("OPEN_AGENTS_PROJECT_ID", "")
	rr := newResolveRoutes()
	rr.addResolve("9", "", http.StatusOK,
		`{"ref":"9","sessionId":"openagents-9","projectId":"openagents","num":9,"matchedBy":"num"}`)
	// An uncertain steer is what makes the CLI print a recovery hint.
	rr.steerReply = resolveReply{status: http.StatusConflict, body: `{"error":"conflict","code":"CHAT_STEER_UNCERTAIN","message":"provider outcome unknown"}`}
	srv := rr.serve(t)
	writeRunFileFor(t, cfg, srv)

	_, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "send", "--session", "9", "--steer", "--message", "hi", "--client-message-id", "handle-1")
	if err == nil {
		t.Fatalf("expected the fake daemon to reject the steer\nstderr=%s", errOut)
	}
	// The recovery hint has to name a session the user can act on, which is the
	// resolved id, never the number they typed.
	if !strings.Contains(err.Error(), "--session openagents-9") {
		t.Errorf("recovery hint did not use the resolved id: %v", err)
	}
}
