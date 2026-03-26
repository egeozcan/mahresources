package application_context

import (
	"fmt"

	"mahresources/models/query_models"
)

// requireIDs returns an error if the ID slice is empty.
func requireIDs(ids []uint, entityName string) error {
	if len(ids) == 0 {
		return fmt.Errorf("at least one %s ID is required", entityName)
	}
	return nil
}

// validateBulkEditQuery validates both ID and EditedId slices are non-empty.
func validateBulkEditQuery(query *query_models.BulkEditQuery, entityName, editedEntityName string) error {
	if err := requireIDs(query.ID, entityName); err != nil {
		return err
	}
	return requireIDs(query.EditedId, editedEntityName)
}
