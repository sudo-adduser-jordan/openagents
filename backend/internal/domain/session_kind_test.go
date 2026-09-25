package domain

import "testing"

func TestParseSessionKind(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want SessionKind
		err  bool
	}{
		{name: "omitted", want: ""},
		{name: "worker", raw: "worker", want: KindWorker},
		{name: "manager", raw: "manager", want: KindManager},
		{name: "unknown", raw: "orchestrator", err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSessionKind(tc.raw)
			if tc.err {
				if err == nil {
					t.Fatalf("ParseSessionKind(%q) succeeded, want error", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSessionKind(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("ParseSessionKind(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
