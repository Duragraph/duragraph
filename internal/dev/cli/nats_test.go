package cli

import "testing"

// TestSubjectFor pins the --aggregate → NATS-subject mapping. This is
// load-bearing: a wrong subject yields "subscribed but no events" with
// no error, which is hard to debug. The mapping must stay in sync with
// internal/infrastructure/messaging/outbox_relay.go's buildTopic.
func TestSubjectFor(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "duragraph.>", true},
		{"run", "duragraph.runs.>", true},
		{"execution", "duragraph.executions.>", true},
		{"thread", "duragraph.events.thread.>", true},
		{"workflow", "duragraph.events.workflow.>", true},
		{"tenant", "duragraph.events.tenant.>", true},
		{"user", "duragraph.events.user.>", true},
		{"bogus", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := SubjectFor(tc.in)
			if tc.ok && err != nil {
				t.Fatalf("SubjectFor(%q): unexpected err %v", tc.in, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("SubjectFor(%q): expected err, got %q", tc.in, got)
			}
			if got != tc.want {
				t.Fatalf("SubjectFor(%q): got %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
