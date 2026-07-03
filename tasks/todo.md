# MRQL v3 Package 3: Similarity Search — active task

Design: `mrql/v3-package-3-similarity-design.md` (semantics confirmed via
AskUserQuestion: runtime-setting default threshold, aHash filter always,
WITHIN>11 = validation error, ORDER BY distance included in v1).

## Plan (TDD: red tests per slice, then green)

- [x] 1. Branch `feat/mrql-package3-similarity`
- [x] 2. Lexer: `TokenSimilarTo` merged token (SIMILAR + TO, ORDER BY precedent);
       `SIMILAR` alone stays identifier. Lexer tests.
- [x] 3. AST + parser: `SimilarToExpr`, `parseSimilarTo` from `parsePrimary`;
       contextual `WITHIN` (identifier, not keyword). Parser tests (shapes + errors).
- [x] 4. Validator: entity=resource only, positive ID, WITHIN 0..11
       (`MaxSimilarityDistance`), ORDER BY `distance` rules (exactly one
       SIMILAR TO). Validator tests.
- [x] 5. Translator: `translateSimilarTo` (UNION ALL both directions, COALESCE
       filter, aHash clause, 1=0 entity fallback), `TranslateOptions`
       thresholds, `resolveOrderByColumn` distance key. Test-schema
       `testResourceSimilarity` + seeded pairs. SQL-shape + execution tests
       (SQLite), PG counterpart.
- [x] 6. application_context: fill options from `similarityThresholds()` at
       every TranslateOptions construction site (incl. plugin adapter);
       constant-sync test vs `hash_worker.MaxStoredPDistance`.
- [x] 7. Completer: SIMILAR TO suggestion (resource-gated), WITHIN after `)`,
       `distance` in ORDER BY context. Completer tests.
- [x] 8. NL generation: prompt rule + examples + prompt test; generation_lint
       case + test.
- [x] 9. Docs: skill mrql.md, docs-site mrql.md + mrql-reference.md.
- [x] 10. E2E: `e2e/tests/mrql-similarity.spec.ts` (syntax + validation-error UX).
- [ ] 11. Full verification: Go unit, build, browser+CLI e2e, PG Go, PG e2e.
- [ ] 12. Self-review the diff (same bar as package 2), fix findings.

## Review

(to be filled at the end)
