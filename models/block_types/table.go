package block_types

import (
	"encoding/json"
	"errors"
)

// tableColumn represents a column definition in a table block.
type tableColumn struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// tableContent represents the content schema for table blocks.
// Either columns+rows OR queryId should be provided, not both.
// Columns can be either simple strings or objects with id/label.
// Rows can be either arrays of values or objects with column IDs as keys.
// QueryParams and IsStatic are only valid when QueryID is set.
type tableContent struct {
	Columns     []json.RawMessage `json:"columns"`
	Rows        []json.RawMessage `json:"rows"`
	QueryID     *uint             `json:"queryId"`
	QueryParams map[string]any    `json:"queryParams,omitempty"`
	IsStatic    bool              `json:"isStatic,omitempty"`
}

// tableState represents the state schema for table blocks.
type tableState struct {
	SortColumn string `json:"sortColumn"`
	SortDir    string `json:"sortDir"`
}

// TableBlockType implements BlockType for table content.
type TableBlockType struct{}

func (t TableBlockType) Type() string {
	return "table"
}

func (t TableBlockType) ValidateContent(content json.RawMessage) error {
	var c tableContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}

	hasManualData := len(c.Columns) > 0 || len(c.Rows) > 0
	hasQueryID := c.QueryID != nil

	if hasManualData && hasQueryID {
		return errors.New("table cannot have both columns/rows and queryId")
	}

	// queryParams and isStatic are only valid when queryId is set
	if !hasQueryID {
		if len(c.QueryParams) > 0 {
			return errors.New("queryParams is only valid when queryId is set")
		}
		if c.IsStatic {
			return errors.New("isStatic is only valid when queryId is set")
		}
	}

	return nil
}

func (t TableBlockType) ValidateState(state json.RawMessage) error {
	var s tableState
	if err := json.Unmarshal(state, &s); err != nil {
		return err
	}
	if s.SortDir != "" && s.SortDir != "asc" && s.SortDir != "desc" {
		return errors.New("sortDir must be 'asc' or 'desc'")
	}
	return nil
}

func (t TableBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"columns": [], "rows": []}`)
}

func (t TableBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(TableBlockType{})
}
