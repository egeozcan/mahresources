package block_types

import (
	"encoding/json"
	"errors"
)

// headingContent represents the content schema for heading blocks.
type headingContent struct {
	Text  string `json:"text"`
	Level int    `json:"level"`
}

// HeadingBlockType implements BlockType for heading content.
type HeadingBlockType struct{}

func (h HeadingBlockType) Type() string {
	return "heading"
}

func (h HeadingBlockType) ValidateContent(content json.RawMessage) error {
	var c headingContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	if c.Level < 1 || c.Level > 6 {
		return errors.New("heading level must be 1-6")
	}
	return nil
}

func (h HeadingBlockType) ValidateState(state json.RawMessage) error {
	// Heading blocks have no state
	return nil
}

func (h HeadingBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"text": "", "level": 2}`)
}

func (h HeadingBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{}`)
}

func init() {
	RegisterBlockType(HeadingBlockType{})
}
