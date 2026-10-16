package cli

import "time"

// evaluateRPO scores one guest's points against one job's runs, for the
// pure-logic tests: every miss counts as proven by a task log, unless
// something else leaves it unproven.
func evaluateRPO(points []restorePoint, runs []int64, start, now time.Time) rpoRow {
	return evaluateJobsRPO(points, []rpoTarget{{jobRuns: [][]int64{runs}, points: points, proof: func(int64, int64) bool { return true }}}, start, now)
}
