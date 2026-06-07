package fleet

import "fmt"

type AgentSignals struct {
	Status         string
	FailingChecks  int
	NeedsReboot    bool
	PatchesPending bool
	PendingActions int
}

func TriageScore(s AgentSignals) int {
	score := 0
	switch s.Status {
	case "offline":
		score += 40
	case "overdue":
		score += 25
	}
	fc := s.FailingChecks
	if fc > 5 {
		fc = 5
	}
	score += fc * 10
	if s.NeedsReboot {
		score += 8
	}
	if s.PatchesPending {
		score += 5
	}
	pa := s.PendingActions
	if pa > 5 {
		pa = 5
	}
	score += pa * 3
	return score
}

func Reasons(s AgentSignals) []string {
	var r []string
	switch s.Status {
	case "offline":
		r = append(r, "offline")
	case "overdue":
		r = append(r, "overdue check-in")
	}
	if s.FailingChecks > 0 {
		r = append(r, fmt.Sprintf("%d failing checks", s.FailingChecks))
	}
	if s.NeedsReboot {
		r = append(r, "needs reboot")
	}
	if s.PatchesPending {
		r = append(r, "patches pending")
	}
	if s.PendingActions > 0 {
		r = append(r, fmt.Sprintf("%d pending actions", s.PendingActions))
	}
	return r
}
