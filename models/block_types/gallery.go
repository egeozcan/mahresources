package block_types

import (
	"encoding/json"
	"errors"
)

// galleryContent represents the content schema for gallery blocks.
type galleryContent struct {
	ResourceIDs []uint `json:"resourceIds"`
}

// galleryState represents the state schema for gallery blocks.
type galleryState struct {
	Layout string `json:"layout"`
}

// GalleryBlockType implements BlockType for gallery/image collection content.
type GalleryBlockType struct{}

func (g GalleryBlockType) Type() string {
	return "gallery"
}

func (g GalleryBlockType) ValidateContent(content json.RawMessage) error {
	var c galleryContent
	if err := json.Unmarshal(content, &c); err != nil {
		return err
	}
	// resourceIds is required but can be empty
	return nil
}

func (g GalleryBlockType) ValidateState(state json.RawMessage) error {
	var s galleryState
	if err := json.Unmarshal(state, &s); err != nil {
		return err
	}
	if s.Layout != "" && s.Layout != "grid" && s.Layout != "list" {
		return errors.New("gallery layout must be 'grid' or 'list'")
	}
	return nil
}

func (g GalleryBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"resourceIds": []}`)
}

func (g GalleryBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{"layout": "grid"}`)
}

func init() {
	RegisterBlockType(GalleryBlockType{})
}
