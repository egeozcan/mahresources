package block_types

import "encoding/json"

// DividerBlockType implements BlockType for divider/separator content.
// Dividers have no content or state validation requirements.
type DividerBlockType struct{}

func (d DividerBlockType) Type() string {
	return "divider"
}

func (d DividerBlockType) ValidateContent(content json.RawMessage) error {
	// Dividers have no content requirements
	return nil
}

func (d DividerBlockType) ValidateState(state json.RawMessage) error {
	// Dividers have no state
	return nil
}

func (d DividerBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{}`)
}

func (d DividerBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(DividerBlockType{})
}
