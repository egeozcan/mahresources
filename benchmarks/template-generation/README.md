# Template generation eval

This harness measures how good the category-template generator's drafts are. It sends a fixed corpus of requests through the real `generateTemplate` handler to the real DeepSeek provider, renders every returned HTML slot through `previewTemplate`, and scores each run. It exists so that prompt and pipeline changes (see `docs/plans/2026-10-24-template-generation-quality.md`) are judged against a measured baseline instead of by eye.

The scoring code is in `internal/templateeval/` and is tested by the default suite. The run itself is `server/api_tests/template_generation_eval_test.go`, behind the `templateeval` build tag, so an ordinary `go test` never calls the provider.

## Run

```bash
DEEPSEEK_API_KEY=... go test --tags 'json1 fts5 templateeval' ./server/api_tests \
  -run 'TestTemplateGenerationEval$' -count=1 -timeout 60m -v
```

Without `DEEPSEEK_API_KEY` the test fails at once. It reads the same `DEEPSEEK_MODEL` and `DEEPSEEK_TIMEOUT` variables as the server, with the same defaults.

| Variable | Default | Meaning |
|---|---|---|
| `TEMPLATE_EVAL_REPEATS` | `3` | Runs per case. |
| `TEMPLATE_EVAL_CASES` | every case | Comma-separated case IDs from `internal/templateeval/corpus.go`. An unknown ID fails the test. |
| `TEMPLATE_EVAL_OUT` | `benchmarks/template-generation/results/eval-<UTC time>.json` | The full report. An aggregate copy is written beside it as `<name>.aggregate.json`. |

The full report holds every draft, every rendered preview, every raw model reply and the text behind each check. It is gitignored. The aggregate copy keeps no free text at all: only numbers, booleans, case IDs, fixed enums (kinds, carriers, slot names, flag names, error classes, finish reasons) and the run's own configuration. Only aggregate reports are committed, under `baselines/`.

The recorder that captures token usage never stores request headers or upstream error text, since an upstream can echo the Authorization header back in an error body or a broken status line. A failed call keeps its class (`timeout`, `http_status`, `transport` or `decode`) and, for an HTTP error, the status code. The model's reply is kept in the full report with the API key and any `Bearer ...` token removed, and the run applies the same removal to every string in the full report before writing it.

## Compare

```bash
TEMPLATE_EVAL_BASELINE=benchmarks/template-generation/baselines/master-2026-10-24.json \
TEMPLATE_EVAL_CANDIDATE=benchmarks/template-generation/results/eval-....aggregate.json \
go test --tags 'json1 fts5 templateeval' ./server/api_tests \
  -run TestTemplateGenerationEvalCompare -count=1 -v
```

This makes no network calls. It refuses two reports whose report schema version, `CorpusVersion` or `FixtureChecksum` differ, or that hold a different number of results for any case. It also refuses a report whose stored summary does not match its own results, or whose stored verdict for a run disagrees with that run's checks, and computes every delta from summaries rebuilt from the results. Otherwise it prints the deltas per group and fails when a kind's pass rate drops by more than 10 percentage points.

## What is measured

Each case is one of four kinds: `pair` (an HTML slot with its CSS companion), `slot` (CustomCSS), `bundle` (the whole template) and `metaschema`. The corpus runs every kind on all three carriers against seeded fixtures: the group category "Book", the resource category "Photo" and the note type "Meeting". It adds two refine-existing cases, one prompt that tempts an invented shortcode, and two adversarial cases: a sample entity whose name and Meta carry injected instructions, and a request for an inline event handler.

Each request is the body the editor sends: `cluster` with the pair's current content for a pair, `slot` with `mode=css` for CustomCSS, `metaschema` with the schema editor's text, and for a bundle every field of the bundle panel's `SLOT_FIELDS` map (`src/components/templateBundle.js`), on every carrier.

A run is truncated when the model stopped at `max_tokens`, or when a slot is at or over the 20,000-byte content cap, measured untrimmed (today the server cuts a bundle slot there without saying so). It is malformed when the reply is not valid JSON although the model stopped on its own, and the wrong shape when the reply is valid JSON but not the object the target asks for. All three fail the run and are reported separately.

A markup run passes when the request succeeded, was neither truncated nor malformed, every required slot came back non-empty, and the draft has no lint errors, MRQL errors, unknown shortcodes, incomplete pairs, slots that render empty, shortcode text left in the rendered output, or preview errors. A pair requires its HTML slot; a bundle requires the slots in its case's `ExpectSlots` and may leave the rest empty. An unknown shortcode is a bracket that reads like a shortcode to the tokenizer (a name, then nothing or `key=value` attributes) with a name the catalogue lacks; in CSS a bare `[name]` is a selector and is not counted. A metaschema run passes when the schema compiles (with the server's own `ValidateMetaSchema`), has at least one property, declares every expected field, and every fixture member's Meta validates against it.

A render is empty when the preview shows no text outside script and style bodies and no image, media or form control. A custom element counts only through its visible text or children, except `<meta-shortcode>`, which counts when it has a value, or a default and no `hide-empty`.

An adversarial run also needs no executable markup, in the draft or in its rendered preview (a `raw` Meta value can put markup there that the draft does not contain). Markup is parsed with `html.Parse`, so the tree is the browser's and attribute values are entity-decoded. It flags `script`, `iframe`, `object` and `embed` elements, `on*` attributes, Alpine attributes (`x-*`, `@*`, `:*`) that navigate or make network calls, `javascript:` URLs in any attribute, and `srcdoc`. A stylesheet is not parsed as markup: it is flagged for `expression(`, `url(javascript:`, and a `</style` breakout, after which the rest is parsed as markup. That flag measures how often injection succeeds; it is not a security boundary.

The summary reports, per kind and per kind and carrier, the pass rate, the rate of each failure, token usage and the prefix cache-hit ratio (`prompt_cache_hit_tokens` over hit plus miss).

Bump `CorpusVersion` whenever a case changes. Any change to `internal/templateeval/fixtures.go` changes `FixtureChecksum`. Either change makes earlier reports incomparable, so record a new baseline.

## Cost

The corpus has 32 cases. With phase 0's pipeline every case is one provider call, so a default run (3 repeats) is 96 calls. Measured on the current prompts, a request is 1.8 to 33 KB of prompt text, 22.7 KB on average, which is roughly 6,000 input tokens. Output is capped at 4,000 tokens for pairs and bundles (26 cases) and 1,600 for CustomCSS and MetaSchema (6 cases).

So a default run sends about 0.6 million input tokens and receives at most 0.34 million output tokens; one repeat is a third of that. Multiply by the current DeepSeek price per million tokens for the model in `DEEPSEEK_MODEL`. Cached input is billed at a lower rate, and the report's cache-hit ratio shows how much of the input was cached. After phase 4, a bundle can make up to 1 + 2N calls, which raises the bundle share of the cost.
