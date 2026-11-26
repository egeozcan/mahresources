package application_context

// reductionColumns is the projection every ordinary load of resource_reductions
// uses (design D1, rule 3). It names every column except the legacy `plan` and
// `extent` documents, which cross the wire only in the migration: on a library
// that has run a whole-library Reduction they are megabytes per row, and a
// SELECT * would ship them on every fence, lease renewal and page.
// TestReductionColumnsCoverEveryColumnButTheLegacyDocuments keeps this list and
// the model in step.
const reductionColumns = "id, created_at, updated_at, name, created_by_user_id, status, matching_mode, " +
	"exclude_external_resources, keep_as_version_identical, keep_as_version_near, winner_rule, " +
	"computed_at, computing_started_at, compute_deadline, compute_job_id, compute_error, version, " +
	"plan_schema, extent_all, plan_generation, compute_generation, settle_op, settle_lease_until, " +
	"compute_lease_until, apply_lease_until, coverage, plan_settings, last_apply"
