package query_models

// CrossGroupCompareQuery defines the query params for side-by-side group comparison.
type CrossGroupCompareQuery struct {
	Group1ID uint `schema:"g1"`
	Group2ID uint `schema:"g2"`
}
