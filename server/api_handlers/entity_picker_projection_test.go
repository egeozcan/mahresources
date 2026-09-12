package api_handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/contracts"
	"mahresources/models"
)

func TestPickerValueDoesNotCopyNoteBodyIntoSelectorState(t *testing.T) {
	note := models.Note{ID: 1, Name: "A note", Description: strings.Repeat("body", 10000), Meta: []byte(`{"status":"active"}`)}
	value := pickerValue(contracts.EntityPickerEntity{ID: note.ID, Name: note.Name, Raw: note})
	require.NotContains(t, value, "Description", "note text belongs to rendering, not lightweight selector state")
	require.Equal(t, map[string]any{"status": "active"}, value["Meta"])
	require.Len(t, note.Description, 40000, "projection must not mutate the rendering entity")
}
