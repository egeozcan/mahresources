package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"mahresources/shortcodes"
)

// A whole template is drafted in a fan-out (phase 4 of
// docs/plans/2026-10-24-template-generation-quality.md): one planning call
// settles a shared design, then one call per section fills an HTML slot with its
// CSS companion, in parallel. A section that fails is left out and reported, so
// one bad reply no longer costs the whole template.

const (
	// templateFanOutInFlight bounds the pair calls one request has in flight.
	templateFanOutInFlight = 5

	// Caps on what the plan may carry into every pair prompt.
	templatePlanClassPrefixBytes = 40
	templatePlanPaletteBytes     = 400
	templatePlanLayoutBytes      = 600
	templatePlanIntentBytes      = 300

	// The placeholder in a disclosed pair message that exists only after the
	// planning call replies. The continuation rule says only that model output
	// fills it.
	templatePlaceholderPlan = "{{model output: design plan}}"

	templateFanOutCustomCSS = "CustomCSS"
)

// templateDesignPlan is the planning call's reply: one design for every section
// of a whole template, shared by the pair calls that follow.
type templateDesignPlan struct {
	ClassPrefix        string                         `json:"classPrefix"`
	Palette            string                         `json:"palette"`
	Layout             string                         `json:"layout"`
	SharedCSSAdditions string                         `json:"sharedCssAdditions"`
	Sections           map[string]templatePlanSection `json:"sections"`
	Explanation        string                         `json:"explanation"`
}

type templatePlanSection struct {
	Fill   bool   `json:"fill"`
	Intent string `json:"intent"`
}

// templateFanOut is the routing decision of a whole-template request: the
// sections it may fill, and whether CustomCSS is among the requested slots.
type templateFanOut struct {
	// bases are the HTML sections in BundleSlots order; each is filled with its
	// companion, base+"CSS".
	bases []string
	// css is true when CustomCSS was requested, so the plan's shared CSS
	// additions can be appended to it.
	css bool
}

// templateFanOutFor applies the routing predicate to the expanded BundleSlots:
// the planner runs when more than one section is requested or CustomCSS is, and
// otherwise the request is the single call. The handler also represents an
// ordinary pair or cluster request as a two-slot bundle, so the target alone
// cannot tell the paths apart. It returns nil for a request that is not a
// fan-out.
func templateFanOutFor(in TemplateGenerationInput) (*templateFanOut, error) {
	if in.Target != TemplateTargetBundle {
		return nil, nil
	}
	fan := &templateFanOut{}
	seen := map[string]bool{}
	for _, slot := range in.BundleSlots {
		switch {
		case slot == templateFanOutCustomCSS:
			fan.css = true
		case strings.HasSuffix(slot, "CSS"):
			// A companion rides with its HTML slot.
		case !seen[slot]:
			seen[slot] = true
			fan.bases = append(fan.bases, slot)
		}
	}
	if len(fan.bases) <= 1 && !fan.css {
		return nil, nil
	}
	if len(fan.bases) == 0 {
		return nil, fmt.Errorf("%w: a CustomCSS-only selection has no section to add CSS to; use the CustomCSS slot's own Generate button", ErrTemplateGenerationBadRequest)
	}
	return fan, nil
}

func (f *templateFanOut) isBase(slot string) bool {
	for _, base := range f.bases {
		if base == slot {
			return true
		}
	}
	return false
}

// clipBytes cuts s to at most n bytes, on a rune boundary.
func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// parseTemplateDesignPlan decodes the planning reply and bounds it: sections
// nobody requested are dropped, lengths are capped, sections the user selected
// are forced to fill, and CSS additions are dropped when CustomCSS was not
// requested, so the pair prompts never offer additions that cannot ship.
func parseTemplateDesignPlan(raw string, in TemplateGenerationInput) (templateDesignPlan, error) {
	var plan templateDesignPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &plan); err != nil {
		return templateDesignPlan{}, fmt.Errorf("%w: the design plan is not valid JSON", ErrTemplateGenerationProvider)
	}
	fan, err := templateFanOutFor(in)
	if err != nil || fan == nil {
		return templateDesignPlan{}, fmt.Errorf("%w: the request is not a fan-out", ErrTemplateGenerationBadRequest)
	}
	plan.ClassPrefix = clipBytes(strings.TrimSpace(plan.ClassPrefix), templatePlanClassPrefixBytes)
	plan.Palette = clipBytes(strings.TrimSpace(plan.Palette), templatePlanPaletteBytes)
	plan.Layout = clipBytes(strings.TrimSpace(plan.Layout), templatePlanLayoutBytes)
	plan.Explanation = clipBytes(strings.TrimSpace(plan.Explanation), MaxTemplateGeneratedExplanationLength)
	plan.SharedCSSAdditions = strings.TrimSpace(plan.SharedCSSAdditions)
	if !fan.css {
		plan.SharedCSSAdditions = ""
	}
	sections := map[string]templatePlanSection{}
	for name, section := range plan.Sections {
		if fan.isBase(name) {
			section.Intent = clipBytes(strings.TrimSpace(section.Intent), templatePlanIntentBytes)
			sections[name] = section
		}
	}
	if in.SlotsSelected {
		for _, base := range fan.bases {
			section := sections[base]
			section.Fill = true
			sections[base] = section
		}
	}
	plan.Sections = sections
	return plan, nil
}

// fillBases lists the sections to generate, in BundleSlots order.
func (f *templateFanOut) fillBases(plan templateDesignPlan) []string {
	var fill []string
	for _, base := range f.bases {
		if plan.Sections[base].Fill {
			fill = append(fill, base)
		}
	}
	return fill
}

// planJSON is the plan as it is shown to the pair calls: struct fields in
// order and map keys sorted, so it is identical across pairs. Fence markers
// inside it are defanged like any untrusted body.
func (p templateDesignPlan) planJSON() string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(p); err != nil {
		// Only strings and a map of structs of strings are marshalled here.
		panic(err)
	}
	return referenceMarkerPattern.ReplaceAllString(strings.TrimSpace(b.String()), "<<$1")
}

// currentTemplateSlots reads a bundle's current content, a JSON map of slot to
// content. Anything else reads as no slots.
func currentTemplateSlots(in TemplateGenerationInput) map[string]string {
	var slots map[string]string
	if err := json.Unmarshal([]byte(in.CurrentContent), &slots); err != nil {
		return nil
	}
	return slots
}

// pairInput is the input of one pair call: its two slots and only their current
// content.
func (f *templateFanOut) pairInput(in TemplateGenerationInput, base string) TemplateGenerationInput {
	pair := in
	pair.BundleSlots = []string{base, base + "CSS"}
	pair.SlotsSelected = false
	pair.CurrentContent = ""
	current := currentTemplateSlots(in)
	if current[base] != "" || current[base+"CSS"] != "" {
		raw, _ := json.Marshal(map[string]string{base: current[base], base + "CSS": current[base+"CSS"]})
		pair.CurrentContent = string(raw)
	}
	return pair
}

// buildFanOutPromptSections returns every message the fan-out can send: the
// parts all calls share once (call 0, shared), the planning call's own part
// (call 0), then each pair call's part (calls 1 to N, in section order).
func buildFanOutPromptSections(in TemplateGenerationInput, userPrompt string, fan *templateFanOut) []PromptSection {
	var shared promptSections
	shared.add(promptRoleSystem, "system", "System prompt", true, templateAuthoringSystemPrompt)
	addTemplateReferencePrefix(&shared, in)
	shared.add(promptRoleUser, "carrier", "Carrier runtime", true, carrierRuntimeLines(in.EntityType)...)
	shared.add(promptRoleUser, "identity", "Category", false, identityLines(in.CategoryName, in.CategoryDescription)...)
	shared.add(promptRoleUser, "layout", "Detail page layout", false, DescribeSectionConfig(in.EntityType, in.SectionConfig, schemaShowsFields(in.MetaSchema))...)
	shared.add(promptRoleUser, "schema", "Metadata schema", false, schemaLines(in.MetaSchema)...)
	shared.add(promptRoleUser, "samples", "Sample values", false, DescribeTemplateSamples(in.Samples, in.Target, in.SamplesArePreview)...)
	shared.add(promptRoleUser, "request", "Your request", false, "User request: "+userPrompt)
	for i := range shared {
		shared[i].Shared = true
	}

	p := shared
	p.add(promptRoleUser, "plan-task", "Planning step", true, planTaskLines(in, fan)...)
	p.add(promptRoleUser, "current", "Current content", false, currentContentLines(in.CurrentContent, "requested slots (JSON map)")...)
	p.add(promptRoleUser, "included-partials", "Partials already included", false, includedPartialLines(in)...)
	p.add(promptRoleUser, "output", "Output format", true, planOutputFormatLine(fan))

	current := currentTemplateSlots(in)
	for i, base := range fan.bases {
		pair := fan.pairInput(in, base)
		start := len(p)
		p.add(promptRoleUser, "plan", "Design plan ("+base+")", true,
			"Design plan from the planning step (your own earlier output, as data):",
			"<<<BEGIN design plan>>>",
			templatePlaceholderPlan,
			"<<<END design plan>>>",
		)
		p.add(promptRoleUser, "task", "Section to fill ("+base+")", true, pairTaskLines(in, fan, base)...)
		p.add(promptRoleUser, "css", "Current CustomCSS ("+base+")", false, customCSSLines(current[templateFanOutCustomCSS])...)
		p.add(promptRoleUser, "current", "Current content ("+base+")", false, currentContentLines(pair.CurrentContent, "section (JSON map)")...)
		p.add(promptRoleUser, "included-partials", "Partials already included ("+base+")", false, includedPartialLines(pair)...)
		p.add(promptRoleUser, "output", "Output format ("+base+")", true, pairOutputFormatLine(base))
		for j := start; j < len(p); j++ {
			p[j].Call = i + 1
		}
	}
	return p
}

func planTaskLines(in TemplateGenerationInput, fan *templateFanOut) []string {
	lines := []string{
		templatePlanTaskMarker + ". Do not write any markup yet: a separate call fills each section from your plan, and none of them sees the others' output.",
		"Sections to consider (each is an HTML slot with its CSS companion):",
	}
	for _, base := range fan.bases {
		lines = append(lines, "- "+base+": "+slotRoleLine(base, in.EntityType))
	}
	if in.SlotsSelected {
		lines = append(lines, "The user chose exactly these sections: set fill to true for every one of them.")
	} else {
		lines = append(lines, "Choose which of these sections the request needs: set fill to true only for those, and false for the rest. A section with fill false is left as it is.")
	}
	if fan.css {
		lines = append(lines, "CustomCSS was requested: sharedCssAdditions holds new CSS rules for styling the sections share, every class starting with your classPrefix. They are appended to the existing CustomCSS, which is never rewritten, so do not repeat existing rules. Use an empty string when nothing shared is needed.")
	} else {
		lines = append(lines, "CustomCSS was not requested and cannot change: sharedCssAdditions must be an empty string, and every style belongs in a section's own CSS companion.")
	}
	return lines
}

func planOutputFormatLine(fan *templateFanOut) string {
	example := fan.bases[0]
	return `Output format: one JSON object with exactly the keys classPrefix, palette, layout, sharedCssAdditions, sections and explanation, like {"classPrefix":"tpl-book","palette":"slate with amber accents","layout":"stacked cards with 1rem gaps","sharedCssAdditions":".tpl-book-card{padding:1rem}","sections":{"` + example + `":{"fill":true,"intent":"a title row with the category name"}},"explanation":"..."}. classPrefix is a short lowercase prefix every class in every section starts with; palette and layout are one or two sentences; sections maps each section field name above to {fill, intent}, where intent is one sentence.`
}

func pairTaskLines(in TemplateGenerationInput, fan *templateFanOut, base string) []string {
	lines := []string{
		"Task: fill the " + base + " section: return " + base + " and its companion " + base + "CSS together, as values in the slots object.",
		"- " + base + ": " + slotRoleLine(base, in.EntityType),
		"- " + base + "CSS: " + slotRoleLine(base+"CSS", in.EntityType),
		"Follow the design plan above: start every class with its classPrefix, use its palette and layout, and carry out this section's intent.",
	}
	if fan.css {
		lines = append(lines, "The plan's shared CSS additions are appended to CustomCSS separately; do not repeat them here. The existing CustomCSS below is read-only.")
	} else {
		lines = append(lines, "CustomCSS cannot be changed in this request. Every new style belongs in "+base+"CSS (or inline styles); the existing CustomCSS below is read-only.")
	}
	return lines
}

func customCSSLines(css string) []string {
	if strings.TrimSpace(css) == "" {
		return []string{"The existing CustomCSS is empty."}
	}
	return append([]string{"The existing CustomCSS (read-only):"}, referenceBlock("current CustomCSS", css)...)
}

func pairOutputFormatLine(base string) string {
	return `Preserve existing content and styles unless the request requires changing them.
Output format: one JSON object with exactly the keys slots and explanation, like {"slots":{"` + base + `":"<h1 class=\"tpl-title\">[property path=\"Name\"]</h1>","` + base + `CSS":".tpl-title{margin:0}"},"explanation":"..."}. slots holds exactly ` + base + ` and ` + base + `CSS; include an empty CSS value when no styles are needed.`
}

// templatePlanTaskMarker opens the planning call's task; the tests and the
// eval harness recognise the call by it.
const templatePlanTaskMarker = "Task: plan the design of a whole template"

// discloseFanOut assembles a fan-out's disclosure: the plan call plus one
// templated pair call per candidate section, and the possible repair
// continuation after them.
func (g *defaultTemplateGenerator) discloseFanOut(in TemplateGenerationInput, prompt string, fan *templateFanOut) templateRequest {
	sections := buildTemplatePromptSections(in, prompt)
	maxPairCalls := len(fan.bases)
	if !g.config.RepairDisabled {
		// Each pair may take one repair; the plan never does. The possible
		// repair call is part of what the user approves.
		maxPairCalls *= 2
		sections = append(sections, templateRepairSectionsAt(len(fan.bases)+1)...)
	}
	plan, pair := g.config.profileFor(TemplateKindPlan), g.config.profileFor(TemplateKindPair)
	disclosure := newTemplateDisclosure(g.provider.Endpoint(), []TemplateDisclosedProfile{
		{Kind: TemplateKindPlan, Model: plan.Model, Thinking: plan.Thinking, MaxTokens: plan.MaxTokens, MaxCalls: 1},
		{Kind: TemplateKindPair, Model: pair.Model, Thinking: pair.Thinking, MaxTokens: pair.MaxTokens, MaxCalls: maxPairCalls},
	}, sections)
	return templateRequest{in: in, prompt: prompt, disclosure: disclosure, fan: fan}
}

// templateUsageTally sums the token usage of every call of one request.
type templateUsageTally struct {
	mu    sync.Mutex
	calls int
	sum   TemplateTokenUsage
}

type templateUsageTallyKey struct{}

func (t *templateUsageTally) add(u TemplateTokenUsage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls++
	t.sum.Prompt += u.Prompt
	t.sum.Completion += u.Completion
	t.sum.Reasoning += u.Reasoning
	t.sum.CacheHit += u.CacheHit
	t.sum.CacheMiss += u.CacheMiss
}

// pairOutcome is one pair call's end: a whole pair, or why there is none.
type pairOutcome struct {
	base   string
	result *TemplateGenerationResult
	repair *TemplateRepair
	// failure is the reason, for the warning, when there is no pair.
	failure string
	err     error
}

func (g *defaultTemplateGenerator) generateFanOut(ctx context.Context, request templateRequest) (*TemplateGenerationResult, error) {
	in, fan, sections := request.in, request.fan, request.disclosure.Sections
	deadline := g.config.BundleDeadline
	if deadline <= 0 {
		deadline = DefaultTemplateBundleDeadline
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	tally := &templateUsageTally{}
	ctx = context.WithValue(ctx, templateUsageTallyKey{}, tally)
	started := g.now()

	planProfile := g.config.profileFor(TemplateKindPlan)
	planReq := TemplateDraftRequest{
		System:    promptMessage(sections, 0, promptRoleSystem),
		User:      promptMessage(sections, 0, promptRoleUser),
		Model:     planProfile.Model,
		Thinking:  planProfile.Thinking,
		MaxTokens: planProfile.MaxTokens,
	}
	planResp, err := g.callProvider(ctx, planProfile, planReq, "plan", in)
	if err != nil {
		return nil, err
	}
	if planResp.FinishReason == "length" {
		return nil, fmt.Errorf("%w: the design plan was cut off", ErrTemplateGenerationProvider)
	}
	plan, err := parseTemplateDesignPlan(planResp.Content, in)
	if err != nil {
		return nil, err
	}
	fill := fan.fillBases(plan)
	if len(fill) == 0 {
		return nil, fmt.Errorf("%w: the design plan chose no sections to fill", ErrTemplateGenerationProvider)
	}

	pairProfile := g.config.profileFor(TemplateKindPair)
	continuation := promptMessage(sections, len(fan.bases)+1, promptRoleUser)
	planText := plan.planJSON()
	outcomes := make([]pairOutcome, len(fill))
	inFlight := make(chan struct{}, templateFanOutInFlight)
	var wg sync.WaitGroup
	for i, base := range fill {
		wg.Add(1)
		go func(i int, base string) {
			defer wg.Done()
			select {
			case inFlight <- struct{}{}:
				defer func() { <-inFlight }()
			case <-ctx.Done():
				outcomes[i] = pairOutcome{base: base, err: templateCallError(ctx, ctx.Err())}
				return
			}
			// A free slot and an expired deadline can both be ready; a pair not
			// started by the deadline counts as failed.
			if err := ctx.Err(); err != nil {
				outcomes[i] = pairOutcome{base: base, err: templateCallError(ctx, err)}
				return
			}
			outcomes[i] = g.generatePair(ctx, in, fan, sections, base, planText, pairProfile, continuation)
		}(i, base)
	}
	wg.Wait()

	result, err := assembleFanOut(in, fan, plan, outcomes)
	g.logFanOut(in, tally, len(fill), result, err, g.now().Sub(started))
	return result, err
}

// generatePair makes one pair call, repairs it once if it has errors, and
// reduces the answer to a whole pair or a failure.
func (g *defaultTemplateGenerator) generatePair(ctx context.Context, in TemplateGenerationInput, fan *templateFanOut, sections []PromptSection, base, planText string, profile TemplateCallProfile, continuation string) pairOutcome {
	call := 0
	for i, candidate := range fan.bases {
		if candidate == base {
			call = i + 1
		}
	}
	pair := fan.pairInput(in, base)
	var own []PromptSection
	for _, section := range sections {
		if section.Shared || section.Call == call {
			if section.Key == "plan" && !section.Shared {
				section.Text = strings.Replace(section.Text, templatePlaceholderPlan, planText, 1)
			}
			own = append(own, section)
		}
	}
	req := TemplateDraftRequest{
		System:    promptMessage(own, call, promptRoleSystem),
		User:      promptMessage(own, call, promptRoleUser),
		Model:     profile.Model,
		Thinking:  profile.Thinking,
		MaxTokens: profile.MaxTokens,
	}
	started := g.now()
	resp, err := g.callProvider(ctx, profile, req, "draft", pair)
	if err != nil {
		return pairOutcome{base: base, err: err}
	}
	draft, err := parseBundleDraft(pair, resp)
	if err != nil {
		return pairOutcome{base: base, err: err}
	}
	draft, repair := g.repair(ctx, pair, profile, req, resp, draft, continuation, started)
	if draft.truncated {
		return pairOutcome{base: base, repair: repair, failure: templateTruncationMessage, err: fmt.Errorf("%w: the reply was cut off", ErrTemplateGenerationProvider)}
	}
	result, err := finalizeDraft(draft)
	if err != nil {
		return pairOutcome{base: base, repair: repair, err: err}
	}
	_, hasHTML := result.Slots[base]
	_, hasCSS := result.Slots[base+"CSS"]
	if !hasHTML || !hasCSS {
		return pairOutcome{base: base, repair: repair, failure: "the reply was too large, malformed or incomplete", err: fmt.Errorf("%w: the reply was unusable", ErrTemplateGenerationProvider)}
	}
	return pairOutcome{base: base, result: result, repair: repair}
}

// failureReason is the sentence for a failed pair's warning.
func (o pairOutcome) failureReason() string {
	switch {
	case o.failure != "":
		return o.failure
	case errors.Is(o.err, ErrTemplateGenerationTimeout):
		return "the request ran out of time"
	default:
		return "the provider returned an error or an unusable reply"
	}
}

// assembleFanOut builds the response from the pair outcomes: whole pairs go in,
// failed ones are listed and warned about, and CustomCSS, if requested, is the
// current CustomCSS followed by the plan's additions.
func assembleFanOut(in TemplateGenerationInput, fan *templateFanOut, plan templateDesignPlan, outcomes []pairOutcome) (*TemplateGenerationResult, error) {
	result := &TemplateGenerationResult{
		Target:      TemplateTargetBundle,
		Slots:       map[string]string{},
		Explanation: plan.Explanation,
		Issues:      []shortcodes.LintIssue{},
		SlotIssues:  map[string][]shortcodes.LintIssue{},
	}
	var repair *TemplateRepair
	timeouts := 0
	for _, outcome := range outcomes {
		if outcome.repair != nil {
			if repair == nil {
				repair = &TemplateRepair{}
			}
			repair.Attempted = repair.Attempted || outcome.repair.Attempted
			repair.Applied = repair.Applied || outcome.repair.Applied
			if repair.Reason == "" {
				repair.Reason = outcome.repair.Reason
			}
		}
		if outcome.result == nil {
			if errors.Is(outcome.err, ErrTemplateGenerationTimeout) {
				timeouts++
			}
			result.FailedSlots = append(result.FailedSlots, outcome.base)
			result.Issues = append(result.Issues, shortcodes.LintIssue{
				Severity: shortcodes.SeverityWarning,
				Message:  fmt.Sprintf("%s could not be generated and was left unchanged: %s", outcome.base, outcome.failureReason()),
			})
			continue
		}
		for slot, content := range outcome.result.Slots {
			result.Slots[slot] = content
		}
		for slot, issues := range outcome.result.SlotIssues {
			result.SlotIssues[slot] = issues
		}
		result.Issues = append(result.Issues, outcome.result.Issues...)
		if result.Explanation == "" {
			result.Explanation = outcome.result.Explanation
		}
	}
	if len(result.FailedSlots) == len(outcomes) {
		if timeouts == len(outcomes) {
			return nil, ErrTemplateGenerationTimeout
		}
		return nil, fmt.Errorf("%w: no section could be generated", ErrTemplateGenerationProvider)
	}
	if fan.css && plan.SharedCSSAdditions != "" {
		css, issues := appendSharedCSS(in, plan.SharedCSSAdditions)
		result.Slots[templateFanOutCustomCSS] = css
		if len(issues) > 0 {
			result.SlotIssues[templateFanOutCustomCSS] = issues
			result.Issues = append(result.Issues, issues...)
		}
	}
	if len(result.SlotIssues) == 0 {
		result.SlotIssues = nil
	}
	result.Valid = !hasErrorIssue(result.Issues)
	result.Repair = repair
	return result, nil
}

// appendSharedCSS is the current CustomCSS followed, byte for byte, by the
// plan's additions, and the lint issues of the additions alone, shifted to
// offsets in the combined slot. Existing rules are never rewritten, so a
// section that failed, was skipped or was not selected keeps the styles it
// depends on.
func appendSharedCSS(in TemplateGenerationInput, additions string) (string, []shortcodes.LintIssue) {
	current := currentTemplateSlots(in)[templateFanOutCustomCSS]
	combined, offset := additions, 0
	if strings.TrimSpace(current) != "" {
		separator := "\n\n"
		if strings.HasSuffix(current, "\n\n") {
			separator = ""
		} else if strings.HasSuffix(current, "\n") {
			separator = "\n"
		}
		combined = current + separator + additions
		offset = len(current) + len(separator)
	}
	var issues []shortcodes.LintIssue
	if len(additions) > MaxTemplateGeneratedContentLength {
		issues = append(issues, templateOversizeIssue(templateFanOutCustomCSS, len(additions)))
	}
	for _, issue := range validateTemplateContent(TemplateTargetSlot, in, additions, true) {
		if issue.End > issue.Start {
			issue.Start += offset
			issue.End += offset
		}
		issues = append(issues, issue)
	}
	return combined, issues
}

// logFanOut writes the request's one summary line: the calls' token usage
// summed, where the cache hit ratio of a whole template is read.
func (g *defaultTemplateGenerator) logFanOut(in TemplateGenerationInput, tally *templateUsageTally, pairs int, result *TemplateGenerationResult, err error, dur time.Duration) {
	tally.mu.Lock()
	calls, u := tally.calls, tally.sum
	tally.mu.Unlock()
	failed, outcome := 0, "ok"
	if result != nil {
		failed = len(result.FailedSlots)
	} else if err != nil {
		outcome = "error"
	}
	g.config.Logf("template generation: phase=fanout kind=bundle carrier=%s calls=%d pairs=%d failed=%d prompt=%d cache_hit=%d cache_miss=%d completion=%d reasoning=%d result=%s dur=%dms",
		in.EntityType, calls, pairs, failed, u.Prompt, u.CacheHit, u.CacheMiss, u.Completion, u.Reasoning, outcome, dur.Milliseconds())
}
