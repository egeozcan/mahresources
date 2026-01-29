package block_types

import (
	"encoding/json"
	"errors"
)

// textContent represents the content schema for text blocks.
type textContent struct {
	Text *string `json:"text"`
}

// TextBlockType implements BlockType for plain text content.
type TextBlockType struct{}

func (t TextBlockType) Type() string {
	return "text"
}

func (t TextBlockType) ValidateContent(content json.RawMessage) error {
	var c textContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	if c.Text == nil {
		return errors.New("text block content must have a 'text' field")
	}
	return nil
}

func (t TextBlockType) ValidateState(state json.RawMessage) error {
	// Text blocks have no state
	return nil
}

func (t TextBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"text": ""}`)
}

func (t TextBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(TextBlockType{})
}
