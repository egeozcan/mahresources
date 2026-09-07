package application_context

import "mahresources/mrql"

func validateCategoryMetadataIndexes(raw *string, entity string) error {
	if raw == nil {
		return nil
	}
	_, err := mrql.ParseMetadataIndexKeys(*raw, entity)
	return err
}

func metadataIndexesValue(raw *string) string {
	if raw == nil {
		return ""
	}
	return *raw
}
