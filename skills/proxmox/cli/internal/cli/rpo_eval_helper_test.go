package cli

import "time"

// evaluateRPO scores one guest's points against one job's runs, for the
// pure-logic tests: every run counts as having a recorded failure, so a miss
// is proven unless something else leaves it unproven.
func evaluateRPO(points []restorePoint, runs []int64, start, now time.Time) rpoRow {
	return evaluateJobsRPO(points, []rpoTarget{{jobRuns: [][]int64{runs}, points: points, failedAt: runs}}, start, now)
}
