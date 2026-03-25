package budgetenforceproc

import "sync"

// costKey identifies a cost bucket for accumulation.
type costKey struct {
	projectID string
	runID     string
	agentName string // empty for project/run-scoped rules
	userID    string // empty for non-user-scoped rules
}

// accumulator tracks running cost per (project, run, agent) tuple.
// It is safe for concurrent access.
type accumulator struct {
	mu    sync.Mutex
	costs map[costKey]float64
}

func newAccumulator() *accumulator {
	return &accumulator{costs: make(map[costKey]float64)}
}

// add increments cost for the given key, returning the new total.
func (a *accumulator) add(key costKey, cost float64) float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.costs[key] += cost
	return a.costs[key]
}

// get returns the current accumulated cost for a key.
func (a *accumulator) get(key costKey) float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.costs[key]
}

// resetRun clears all cost buckets for a given run.
// Called after a halt alert to prevent repeated alerts.
func (a *accumulator) resetRun(projectID, runID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.costs {
		if k.projectID == projectID && k.runID == runID {
			delete(a.costs, k)
		}
	}
}

// resetUser clears all cost buckets for a given user.
// Called after a user-scoped halt alert to prevent repeated alerts.
func (a *accumulator) resetUser(projectID, userID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.costs {
		if k.projectID == projectID && k.userID == userID {
			delete(a.costs, k)
		}
	}
}

// alertKey uniquely identifies a (rule, run) pair that has already fired.
type alertKey struct {
	ruleID string
	runID  string
}

// alertDedup prevents re-firing the same alert for the same run.
type alertDedup struct {
	mu   sync.Mutex
	seen map[alertKey]struct{}
}

func newAlertDedup() *alertDedup {
	return &alertDedup{seen: make(map[alertKey]struct{})}
}

// check returns true if this is the first time we've seen (ruleID, runID),
// and marks it as seen.
func (d *alertDedup) check(ruleID, runID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	k := alertKey{ruleID: ruleID, runID: runID}
	if _, alreadySeen := d.seen[k]; alreadySeen {
		return false
	}
	d.seen[k] = struct{}{}
	return true
}
