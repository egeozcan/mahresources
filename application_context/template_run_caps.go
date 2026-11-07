package application_context

import "time"

// TemplateRunCaps are the limits a harness generator runs under: the Turn cap,
// the run deadline and the token budget.
type TemplateRunCaps struct {
	MaxTurns    int
	Deadline    time.Duration
	TokenBudget int
}

// RunCaps reports the run limits of a harness generator, so the HTTP layer can
// tell the client the caps in the first event of a stream. A classic generator
// has none (all zero).
func (g *defaultTemplateGenerator) RunCaps() TemplateRunCaps {
	if !g.harness || g.config.Harness == nil {
		return TemplateRunCaps{}
	}
	s := g.config.Harness.withDefaults()
	return TemplateRunCaps{MaxTurns: s.MaxTurns, Deadline: s.Deadline, TokenBudget: s.TokenBudget}
}
