//go:build postgres && json1 && fts5

package application_context

import "testing"

// The Postgres wrappers of the scale proofs (see reduction_scale_test.go). Each
// scenario gets a fresh container database.

func newScalePGContext(t *testing.T) *MahresourcesContext { return newOpsPGContext(t) }

func TestScaleReviewPG(t *testing.T)         { runScaleReview(t, newScalePGContext) }
func TestScaleListAndBulkPG(t *testing.T)    { runScaleListAndBulk(t, newScalePGContext) }
func TestScaleApplyPG(t *testing.T)          { runScaleApply(t, newScalePGContext) }
func TestScaleDeleteAndMergePG(t *testing.T) { runScaleDeleteAndMerge(t, newScalePGContext) }
func TestScaleComputePG(t *testing.T)        { runScaleCompute(t, newScalePGContext) }
func TestScaleMigrationPG(t *testing.T)      { runScaleMigration(t, newScalePGContext) }
