// Package block_types provides the extensible block type system for the block editor.
// Block types define how different types of content (text, images, code, etc.) are
// validated and handled within documents.
package block_types

import "encoding/json"

// BlockType defines a block type's behavior and validation.
// Each block type (text, image, code, etc.) implements this interface
// to provide its own validation logic and default values.
type BlockType interface {
	// Type returns the unique identifier for this block type (e.g., "text", "image", "code")
	Type() string

	// ValidateContent checks if the content JSON is valid for this block type.
	// Returns an error if the content doesn't match the expected schema.
	ValidateContent(content json.RawMessage) error

	// ValidateState checks if the state JSON is valid for this block type.
	// State contains UI-related data like collapsed state, selection, etc.
	// Returns an error if the state doesn't match the expected schema.
	ValidateState(state json.RawMessage) error

	// DefaultContent returns the initial content for new blocks of this type.
	// This is used when creating a new block without explicit content.
	DefaultContent() json.RawMessage

	// DefaultState returns the initial state for new blocks of this type.
	// This is used when creating a new block without explicit state.
	DefaultState() json.RawMessage
}
