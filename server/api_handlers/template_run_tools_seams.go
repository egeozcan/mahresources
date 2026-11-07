package api_handlers

import (
	"context"

	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/shortcodes"
)

// processTemplateToolSource renders src under the RenderBudget on ctx and
// reports a budget or cancellation stop as an error, so the model never reads
// a cut-off render as what its Draft produces.
func processTemplateToolSource(ctx context.Context, src string, mctx shortcodes.MetaShortcodeContext, renderer shortcodes.PluginRenderer, executor shortcodes.QueryExecutor) (string, error) {
	return shortcodes.ProcessChecked(ctx, src, mctx, renderer, executor)
}

// diffTemplateRevisionsBounded diffs two revisions of a carrier under the
// bounded table, stopping when ctx ends. Callers hold the run's single diff
// slot.
func diffTemplateRevisionsBounded(ctx context.Context, mc *application_context.MahresourcesContext, carrierType string, carrierID, from uint, to *uint) (*contracts.TemplateRevisionDiff, error) {
	return mc.DiffTemplateRevisionsContext(ctx, carrierType, carrierID, from, to)
}
