package block_types

import (
	"encoding/json"
	"fmt"
)

// todoItem represents a single todo item in the content.
type todoItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// todosContent represents the content schema for todos blocks.
type todosContent struct {
	Items []todoItem `json:"items"`
}

// todosState represents the state schema for todos blocks.
type todosState struct {
	Checked []string `json:"checked"`
}

// TodosBlockType implements BlockType for todo list content.
type TodosBlockType struct{}

func (t TodosBlockType) Type() string {
	return "todos"
}

func (t TodosBlockType) ValidateContent(content json.RawMessage) error {
	var c todosContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	// Validate that each item has an id
	for i, item := range c.Items {
		if item.ID == "" {
			return fmt.Errorf("todo item at index %d must have an id", i)
		}
	}
	return nil
}

func (t TodosBlockType) ValidateState(state json.RawMessage) error {
	var s todosState
	if err := json.Unmarshal(state, &s); err != nil {
		return err
	}
	// checked is an array of item IDs that are checked
	return nil
}

// NormalizeTodosState rebuilds a client-submitted todos state as
// {"checked": [...]}, keeping only the checked IDs that are items of content,
// de-duplicated in their original order. Unknown keys are dropped. The result
// is bounded by content, not by the client body, so it is what the share
// server stores.
func NormalizeTodosState(content, state json.RawMessage) (json.RawMessage, error) {
	var c todosContent
	if err := json.Unmarshal(content, &c); err != nil {
		return nil, err
	}
	var s todosState
	if err := json.Unmarshal(state, &s); err != nil {
		return nil, err
	}

	itemIDs := make(map[string]bool, len(c.Items))
	for _, item := range c.Items {
		itemIDs[item.ID] = true
	}

	kept := []string{}
	seen := make(map[string]bool, len(s.Checked))
	for _, id := range s.Checked {
		if itemIDs[id] && !seen[id] {
			seen[id] = true
			kept = append(kept, id)
		}
	}
	return json.Marshal(todosState{Checked: kept})
}

func (t TodosBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"items": []}`)
}

func (t TodosBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{"checked": []}`)
}

func init() {
	RegisterBlockType(TodosBlockType{})
}
