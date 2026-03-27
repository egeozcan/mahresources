package application_context

import (
	"fmt"
)

// requireIDs returns an error if the ID slice is empty.
func requireIDs(ids []uint, entityName string) error {
	if len(ids) == 0 {
		return fmt.Errorf("at least one %s ID is required", entityName)
	}
	return nil
}

