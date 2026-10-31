package server

import "mahresources/models"

// templateRevisionCarriers lists the Template Carrier types that have revision
// routes. It is the one table the router, the OpenAPI registry and the
// authorization policy read, so a carrier cannot be routed but left ungated.
var templateRevisionCarriers = []struct {
	carrierType string
	// apiPrefix is the carrier's /v1 path; revision routes hang off it.
	apiPrefix string
	// pagePrefix is the carrier's HTML path; its History page is
	// pagePrefix + "/revisions?id=<id>".
	pagePrefix string
	// tag and noun are OpenAPI wording.
	tag, noun, idPrefix string
	// edit is the capability the carrier needs to be edited, and therefore to
	// see or restore its history.
	edit capability
}{
	{models.TemplateCarrierCategory, "/v1/category", "/category", "categories", "category", "Category", capTaxonomy},
	{models.TemplateCarrierResourceCategory, "/v1/resourceCategory", "/resourceCategory", "resourceCategories", "resource category", "ResourceCategory", capTaxonomy},
	{models.TemplateCarrierNoteType, "/v1/noteType", "/noteType", "notes", "note type", "NoteType", capEditor},
}
