package block_types

import "encoding/json"

// referencesContent represents the content schema for references blocks.
type referencesContent struct {
	GroupIDs []uint `json:"groupIds"`
}

// ReferencesBlockType implements BlockType for group references content.
type ReferencesBlockType struct{}

func (r ReferencesBlockType) Type() string {
	return "references"
}

func (r ReferencesBlockType) ValidateContent(content json.RawMessage) error {
	var c referencesContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	// groupIds is required but can be empty
	return nil
}

func (r ReferencesBlockType) ValidateState(state json.RawMessage) error {
	// References blocks have no state
	return nil
}

func (r ReferencesBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"groupIds": []}`)
}

func (r ReferencesBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(ReferencesBlockType{})
}
