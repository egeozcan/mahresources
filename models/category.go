package models

import "gorm.io/gorm"

type Category struct {
	gorm.Model
	Name        string  `gorm:"index"`
	Description string  `gorm:"index"`
	Groups      []Group `gorm:"foreignKey:CategoryId"`
}
