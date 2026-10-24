package application_context

import (
	"errors"

	"mahresources/mrql"
	"mahresources/shortcodes"
)

// MRQLStaticEnv is what a template query's verdict depends on besides the
// query and its shortcode: the SQL dialect (regex is PostgreSQL-only) and
// whether full-text search is available (RANK needs it).
type MRQLStaticEnv struct {
	Postgres bool
	FTS      bool
}

// MRQLStaticEnv is this deployment's environment for CheckTemplateMRQL.
func (ctx *MahresourcesContext) MRQLStaticEnv() MRQLStaticEnv {
	return MRQLStaticEnv{Postgres: ctx.db.Config.Dialector.Name() == "postgres", FTS: ctx.ftsAvailable()}
}

// ValidateMRQLInteractiveBounds checks a query's limit, offset and bucket
// limit against the bounds an interactive render enforces.
func ValidateMRQLInteractiveBounds(q *mrql.Query) error {
	return validateMRQLExecutionBounds(q, interactiveMRQLPolicy)
}

// CheckTemplateMRQL gives a template query the verdict rendering it would
// give, without a database. It mirrors the shortcode executor
// (server/template_handlers/template_filters/shortcode_query_executor.go):
// parse, bind the param-* values (a missing or an extra one is an error),
// validate, apply the shortcode's limit and buckets, then the interactive
// bounds, and build the SQL the way ExplainMRQLWithOptions does, through the
// aggregated, bucketed, single-entity and cross-entity paths, on
// mrql.StaticDB. A cross-entity query fails only when every entity fails.
//
// What depends on stored data is out of reach: whether a SCOPE group exists,
// or a scope name is ambiguous, is decided at render time. Only the shape of
// a SCOPE literal is checked here.
func CheckTemplateMRQL(query string, opts shortcodes.MRQLCheckOptions, env MRQLStaticEnv) error {
	parsed, err := mrql.Parse(query)
	if err != nil {
		return err
	}
	var params map[string]any
	if len(opts.Params) > 0 {
		params = make(map[string]any, len(opts.Params))
		for k, v := range opts.Params {
			params[k] = v
		}
	}
	if err := mrql.BindParams(parsed, params); err != nil {
		return err
	}
	if err := mrql.Validate(parsed); err != nil {
		return err
	}
	if opts.Limit > 0 {
		parsed.Limit = opts.Limit
	}
	if err := mrql.CheckScopeLiteral(parsed); err != nil {
		return err
	}
	entityType := mrql.ExtractEntityType(parsed)
	if parsed.GroupBy != nil {
		if entityType == mrql.EntityUnspecified {
			return errors.New("GROUP BY requires an explicit entity type")
		}
		parsed.EntityType = entityType
		if opts.Buckets > 0 {
			parsed.BucketLimit = opts.Buckets
		}
	}
	if parsed.Limit < 0 {
		parsed.Limit = MaxMRQLInteractiveLimit
	}
	if err := ValidateMRQLInteractiveBounds(parsed); err != nil {
		return err
	}
	if mrql.ContainsRegexOperator(parsed) && !env.Postgres {
		return &mrql.TranslateError{Message: "regex match (~*) requires PostgreSQL", Pos: 0}
	}

	db := mrql.StaticDB(env.Postgres)
	fts := env.FTS
	translateOpts := mrql.TranslateOptions{FTSAvailable: &fts}
	switch {
	case parsed.GroupBy != nil && len(parsed.GroupBy.Aggregates) > 0:
		_, err = mrql.BuildAggregatedGroupBy(parsed, db, translateOpts)
		return err
	case parsed.GroupBy != nil:
		if parsed.Limit > maxBucketedTotalItems {
			parsed.Limit = maxBucketedTotalItems
		}
		_, err = mrql.BuildGroupByKeys(parsed, db, translateOpts)
		return err
	case entityType != mrql.EntityUnspecified:
		parsed.EntityType = entityType
		_, err = mrql.TranslateWithOptions(parsed, db, translateOpts)
		return err
	default:
		_, _, perEntityCap, err := crossEntityWindow(parsed, MaxMRQLInteractiveLimit, interactiveMRQLPolicy)
		if err != nil {
			return err
		}
		var first error
		for _, et := range crossEntityTypes {
			branch := crossEntitySelectQuery(parsed, et, perEntityCap)
			if _, err := mrql.TranslateWithOptions(&branch, db, translateOpts); err == nil {
				return nil
			} else if first == nil {
				first = err
			}
		}
		return first
	}
}
