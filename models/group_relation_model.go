package models

import (
	"time"
)

type GroupRelationType struct {
	ID             uint      `gorm:"primarykey"`
	CreatedAt      time.Time `gorm:"index"`
	UpdatedAt      time.Time `gorm:"index"`
	Name           string    `gorm:"uniqueIndex:unique_rel_type"`
	Description    string
	FromCategory   *Category          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	FromCategoryId *uint              `gorm:"uniqueIndex:unique_rel_type"`
	ToCategory     *Category          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ToCategoryId   *uint              `gorm:"uniqueIndex:unique_rel_type"`
	BackRelation   *GroupRelationType `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	BackRelationId *uint
}

type GroupRelation struct {
	ID             uint `gorm:"primarykey"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Name           string
	Description    string
	FromGroup      *Group             `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	FromGroupId    *uint              `gorm:"uniqueIndex:unique_rel"`
	ToGroup        *Group             `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ToGroupId      *uint              `gorm:"uniqueIndex:unique_rel,check:ToGroupId <> FromGroupId"`
	RelationType   *GroupRelationType `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	RelationTypeId *uint              `gorm:"uniqueIndex:unique_rel"`
}
