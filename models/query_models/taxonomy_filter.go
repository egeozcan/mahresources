package query_models

// SingleTaxonomy returns the one taxonomy a list constrains, or zero if unknown.
// Singular and plural filters are intersected by the database scopes.
func SingleTaxonomy(singular uint, plural []uint) uint {
	if singular != 0 {
		return singular
	}
	var id uint
	for _, candidate := range plural {
		if candidate == 0 || (id != 0 && id != candidate) {
			return 0
		}
		id = candidate
	}
	return id
}
