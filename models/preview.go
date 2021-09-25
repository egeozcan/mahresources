package models

import (
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

type Preview struct {
	gorm.Model
	Data        []byte
	ContentType string
	OwnerHash   string
}
