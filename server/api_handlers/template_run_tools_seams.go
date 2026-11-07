package api_handlers

import (
	"context"

	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/shortcodes"
)

// The two calls whose bounded forms belong to lane B1 (shortcodes) are reached
// through these functions, so merging that lane means changing only their
// bodies: processTemplateToolSource becomes shortcodes.ProcessChecked, and
// diffTemplateRevisionsBounded the bounded revision diff.

// processTemplateToolSource renders src and reports a budget or cancellation
// stop as an error. Until the render budget is enforced inside shortcodes, a
// cancelled context is the one stop it can see.
func processTemplateToolSource(ctx context.Context, src string, mctx shortcodes.MetaShortcodeContext, renderer shortcodes.PluginRenderer, executor shortcodes.QueryExecutor) (string, error) {
	out := shortcodes.Process(ctx, src, mctx, renderer, executor)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return out, nil
}

// diffTemplateRevisionsBounded diffs two revisions of a carrier. Callers hold
// the run's single diff slot.
func diffTemplateRevisionsBounded(ctx context.Context, mc *application_context.MahresourcesContext, carrierType string, carrierID, from uint, to *uint) (*contracts.TemplateRevisionDiff, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return mc.DiffTemplateRevisions(carrierType, carrierID, from, to)
}
