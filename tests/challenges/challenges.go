// Package challenges defines a lightweight scenario harness for
// My-Patreon-Manager — the project-specific counterpart to the generic
// `digital.vasic.challenges` framework carried in the Challenges/
// submodule.
//
// A Scenario captures one named operator workflow (e.g. "drift halts
// publish", "credential-bearing URL is redacted end-to-end") and runs
// an assertion block against real project code. Scenarios are
// registered by name, executed from `challenges_test.go`, and their
// passing/failing state is reported as regular Go test results so the
// existing test runner, race detector, and coverage tooling all
// understand them.
//
// Why not import the upstream framework directly? The Challenges
// submodule depends on a nested Containers module and Podman-rootless
// container runtimes; most project-specific scenarios here don't need
// that machinery. If deeper orchestration becomes necessary, the
// upstream framework is a drop-in replacement — Scenario's shape
// matches BaseChallenge's Name/Execute/Assertion semantics.
package challenges

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Scenario is a single project-specific integration case. Execute
// returns a Report describing what happened; Assert converts that
// report into test passes/failures.
type Scenario struct {
	Name        string
	Description string
	Execute     func(ctx context.Context) (*Report, error)
	Assert      func(t *testing.T, r *Report)
}

// Report captures the outcome of a single scenario execution.
// Outputs is a free-form bag scenarios use to pass data to their
// assert phase without leaking internals through struct fields.
type Report struct {
	Scenario string
	Outputs  map[string]any
	Started  time.Time
	Duration time.Duration
}

// Registry keeps scenarios indexed by name. A single global Registry
// is created via newRegistry() in challenges_test.go; callers never
// share it across test binaries.
type Registry struct {
	scenarios []Scenario
}

// NewRegistry returns an empty Registry ready to receive scenarios.
func NewRegistry() *Registry {
	return &Registry{scenarios: []Scenario{}}
}

// Register adds a scenario to the registry. Duplicate names surface
// as an error rather than silent overwrite so missing-assertion
// regressions can't disguise themselves as "the new scenario ran".
func (r *Registry) Register(s Scenario) error {
	for _, existing := range r.scenarios {
		if existing.Name == s.Name {
			return fmt.Errorf("challenges: duplicate scenario %q", s.Name)
		}
	}
	if s.Name == "" {
		return fmt.Errorf("challenges: scenario requires non-empty Name")
	}
	if s.Execute == nil {
		return fmt.Errorf("challenges: scenario %q requires non-nil Execute", s.Name)
	}
	if s.Assert == nil {
		return fmt.Errorf("challenges: scenario %q requires non-nil Assert", s.Name)
	}
	r.scenarios = append(r.scenarios, s)
	return nil
}

// MustRegister panics if Register returns an error. Use from init()
// functions where duplicate-name bugs should crash the test binary
// immediately rather than silently producing wrong behavior.
func (r *Registry) MustRegister(s Scenario) {
	if err := r.Register(s); err != nil {
		panic(err)
	}
}

// Scenarios returns the registered scenarios in registration order.
func (r *Registry) Scenarios() []Scenario {
	out := make([]Scenario, len(r.scenarios))
	copy(out, r.scenarios)
	return out
}

// Run executes one scenario and captures its Report. Assert runs as
// a t.Run subtest so individual scenario failures don't hide behind
// each other's output.
func (r *Registry) Run(t *testing.T, s Scenario) {
	t.Run(s.Name, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		started := time.Now()
		report, err := s.Execute(ctx)
		if err != nil {
			t.Fatalf("Execute(%s): %v", s.Name, err)
		}
		if report == nil {
			t.Fatalf("Execute(%s): returned nil report", s.Name)
		}
		report.Scenario = s.Name
		report.Started = started
		report.Duration = time.Since(started)

		s.Assert(t, report)
	})
}

// RunAll executes every scenario in the registry. Each scenario is
// isolated via t.Run so one failure doesn't abort the rest.
func (r *Registry) RunAll(t *testing.T) {
	for _, s := range r.scenarios {
		r.Run(t, s)
	}
}
