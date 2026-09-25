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

func managerCommandServer(t *testing.T) (*httptest.Server, *sessionRequestLog) {
	t.Helper()
	log := &sessionRequestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.append(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/managers":
			_, _ = io.WriteString(w, `{"sessions":[`+
				sessionJSON("other-orch", "other", "manager", "idle", false)+`,`+
				sessionJSON("demo-worker", "demo", "worker", "working", false)+`,`+
				sessionJSON("demo-orch", "demo", "manager", "working", false)+`]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, log
}

func TestManagerList_TableOutput(t *testing.T) {
	cfg := setConfigEnv(t)
	srv, log := managerCommandServer(t)
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "manager", "ls")
	if err != nil {
		t.Fatalf("manager ls failed: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "demo:") || !strings.Contains(out, "demo-orch") {
		t.Fatalf("output missing demo manager:\n%s", out)
	}
	if !strings.Contains(out, "other:") || !strings.Contains(out, "other-orch") {
		t.Fatalf("output missing other manager:\n%s", out)
	}
	if strings.Contains(out, "demo-worker") {
		t.Fatalf("worker session should not be shown in manager ls:\n%s", out)
	}
	want := []string{"GET /api/v1/managers"}
	if got := log.all(); !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %#v, want %#v", got, want)
	}
}

func TestManagerList_JSONOutputDecodes(t *testing.T) {
	cfg := setConfigEnv(t)
	srv, _ := managerCommandServer(t)
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{
		ProcessAlive: func(int) bool { return true },
	}, "manager", "ls", "--json")
	if err != nil {
		t.Fatalf("manager ls --json failed: %v\nstderr=%s", err, errOut)
	}
	var got managerListOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("manager ls --json output is not decodable: %v\noutput=%s", err, out)
	}
	if len(got.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2; data=%#v", len(got.Data), got.Data)
	}
	if got.Data[0].ID != "demo-orch" || got.Data[0].ProjectID != "demo" || got.Data[0].Role != "manager" {
		t.Fatalf("unexpected first JSON entry: %#v", got.Data[0])
	}
	if got.Data[1].ID != "other-orch" || got.Data[1].ProjectID != "other" || got.Data[1].Role != "manager" {
		t.Fatalf("unexpected second JSON entry: %#v", got.Data[1])
	}
}
