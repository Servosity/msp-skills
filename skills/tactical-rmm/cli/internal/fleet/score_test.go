package fleet

import "testing"

func TestTriageScore(t *testing.T) {
	cases := []struct {
		name string
		sig  AgentSignals
		want int
	}{
		{"healthy", AgentSignals{Status: "online"}, 0},
		{"offline", AgentSignals{Status: "offline"}, 40},
		{"overdue_reboot", AgentSignals{Status: "overdue", NeedsReboot: true}, 33},
		{"failing", AgentSignals{Status: "online", FailingChecks: 2}, 20},
		{"capped", AgentSignals{Status: "online", FailingChecks: 99}, 50},
	}
	for _, c := range cases {
		if got := TriageScore(c.sig); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
	if TriageScore(AgentSignals{Status: "offline"}) <= TriageScore(AgentSignals{Status: "online"}) {
		t.Error("offline should outrank healthy")
	}
}

func TestReasons(t *testing.T) {
	if got := Reasons(AgentSignals{Status: "offline", FailingChecks: 1, NeedsReboot: true}); len(got) != 3 {
		t.Errorf("want 3 reasons, got %v", got)
	}
	if got := Reasons(AgentSignals{Status: "online"}); len(got) != 0 {
		t.Errorf("healthy agent should have no reasons, got %v", got)
	}
}
