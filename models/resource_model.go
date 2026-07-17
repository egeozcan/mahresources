package models

import (
	"mahresources/models/types"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Resource struct {
	ID                 uint      `gorm:"primarykey"`
	CreatedAt          time.Time `gorm:"index"`
	UpdatedAt          time.Time `gorm:"index"`
	CreatedByUserId    *uint     `gorm:"index" json:"createdByUserId,omitempty"`
	GUID               *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
	Name               string    `gorm:"index;index:idx_resources_lower_name,expression:LOWER(name)"`
	OriginalName       string    `gorm:"index"`
	OriginalLocation   string    `gorm:"index"`
	Hash               string    `gorm:"index"`
	HashType           string    `gorm:"index"`
	Location           string    `gorm:"index"`
	StorageLocation    *string
	Description        string
	Meta               types.JSON
	Width              uint              `gorm:"index"`
	Height             uint              `gorm:"index"`
	FileSize           int64             `gorm:"index"`
	Category           string            `gorm:"index"`
	ContentType        string            `gorm:"index"`
	ContentCategory    string            `gorm:"index"`
	ResourceCategoryId uint              `gorm:"index;not null;default:1" json:"resourceCategoryId"`
	ResourceCategory   *ResourceCategory `gorm:"constraint:OnUpdate:CASCADE;" json:"resourceCategory,omitempty"`
	SeriesID           *uint             `gorm:"index" json:"seriesId"`
	Series             *Series           `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"series,omitempty"`
	OwnMeta            types.JSON        `json:"ownMeta"`
	Tags               []*Tag            `gorm:"many2many:resource_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Notes              []*Note           `gorm:"many2many:resource_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups             []*Group          `gorm:"many2many:groups_related_resources;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Owner              *Group            `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	OwnerId            *uint             `gorm:"index"`
	Previews           []*Preview        `gorm:"foreignKey:ResourceId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CurrentVersionID   *uint             `json:"currentVersionId"`
	CurrentVersion     *ResourceVersion  `gorm:"foreignKey:CurrentVersionID" json:"currentVersion,omitempty"`
	Versions           []ResourceVersion `gorm:"foreignKey:ResourceID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"versions,omitempty"`

	// BH-037: 1:1 reverse side of ImageHash.Resource, exposed so the resource
	// detail page can surface the perceptual DHash/AHash for observability.
	// ImageHash.ResourceId is the FK on the image_hashes table; the uniqueIndex
	// there guarantees at most one row per resource.
	ImageHash *ImageHash `gorm:"foreignKey:ResourceId;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"imageHash,omitempty"`

	// RenderedHTML is a transient field populated by the API when render=1 is set.
	RenderedHTML string `gorm:"-" json:"renderedHTML,omitempty"`

	// SimilarityDistance is a transient field populated by GetSimilarResources with
	// the perceptual distance (COALESCE(p_distance, hamming_distance)) between this
	// resource and the one the list was queried for. Nil when not part of a
	// similarity result. Drives the confidence-tier badge on the detail page.
	SimilarityDistance *int `gorm:"-" json:"similarityDistance,omitempty"`
}

// SimilarityTier returns a coarse confidence label for a similarity distance:
// 0-2 = near-certain duplicate, 3+ = similar. Returns "" when distance is nil,
// which templates use as the guard (avoids pointer-truthiness pitfalls, so the
// badge still renders for an exact duplicate at distance 0).
func (r *Resource) SimilarityTier() string {
	if r.SimilarityDistance == nil {
		return ""
	}
	if *r.SimilarityDistance <= 2 {
		return "duplicate"
	}
	return "similar"
}

// SimilarityDistanceValue returns the similarity distance for display, or -1 when
// unset. Templates render this rather than the pointer directly.
func (r *Resource) SimilarityDistanceValue() int {
	if r.SimilarityDistance == nil {
		return -1
	}
	return *r.SimilarityDistance
}

func (r *Resource) BeforeCreate(tx *gorm.DB) error {
	if r.GUID == nil {
		guid := types.NewUUIDv7()
		r.GUID = &guid
	}
	return nil
}

func (r Resource) GetCleanLocation() string {
	return filepath.FromSlash(strings.ReplaceAll(r.Location, "\\", "/"))
}

func (r Resource) GetId() uint {
	return r.ID
}

func (r Resource) GetName() string {
	return r.Name
}

func (r Resource) GetDescription() string {
	return r.Description
}

func (r Resource) IsImage() bool {
	return strings.HasPrefix(r.ContentType, "image/")
}

func (r Resource) IsVideo() bool {
	return strings.HasPrefix(r.ContentType, "video/")
}
