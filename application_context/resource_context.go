package application_context

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gabriel-vasile/mimetype"
	"github.com/nfnt/resize"
	"github.com/spf13/afero"
	"gorm.io/gorm/clause"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"path"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/png"
)

func (ctx *MahresourcesContext) GetResource(id uint) (*models.Resource, error) {
	var resource models.Resource

	return &resource, ctx.db.Preload(clause.Associations).First(&resource, id).Error
}

func (ctx *MahresourcesContext) GetResourceCount(query *query_models.ResourceQuery) (int64, error) {
	var resource models.Resource
	var count int64

	return count, ctx.db.Scopes(database_scopes.ResourceQuery(query)).Model(&resource).Count(&count).Error
}

func (ctx *MahresourcesContext) GetResources(offset, maxResults int, query *query_models.ResourceQuery) (*[]models.Resource, error) {
	var resources []models.Resource

	return &resources, ctx.db.Scopes(database_scopes.ResourceQuery(query)).Limit(maxResults).Offset(offset).Preload("Tags").Find(&resources).Error
}

func (ctx *MahresourcesContext) GetResourcesWithIds(ids *[]uint) (*[]*models.Resource, error) {
	var resources []*models.Resource

	if len(*ids) == 0 {
		return &resources, nil
	}

	return &resources, ctx.db.Find(&resources, ids).Error
}

func (ctx *MahresourcesContext) EditResource(resourceQuery *query_models.ResourceEditor) (*models.Resource, error) {
	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var resource models.Resource

	if err := tx.Preload(clause.Associations).First(&resource, resourceQuery.ID).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(&resource).Association("Groups").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(&resource).Association("Tags").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(&resource).Association("Notes").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	groups := make([]models.Group, len(resourceQuery.Groups))
	for i, v := range resourceQuery.Groups {
		groups[i] = models.Group{
			ID: v,
		}
	}

	if err := tx.Model(&resource).Association("Groups").Append(&groups); err != nil {
		tx.Rollback()
		return nil, err
	}

	notes := make([]models.Note, len(resourceQuery.Notes))
	for i, v := range resourceQuery.Notes {
		notes[i] = models.Note{
			ID: v,
		}
	}

	if err := tx.Model(&resource).Association("Notes").Append(&notes); err != nil {
		tx.Rollback()
		return nil, err
	}

	tags := make([]models.Tag, len(resourceQuery.Tags))
	for i, v := range resourceQuery.Tags {
		tags[i] = models.Tag{
			ID: v,
		}
	}

	if err := tx.Model(&resource).Association("Tags").Append(&tags); err != nil {
		tx.Rollback()
		return nil, err
	}

	resource.Name = resourceQuery.Name
	if resourceQuery.Meta != "" {
		resource.Meta = []byte(resourceQuery.Meta)
	}
	resource.Description = resourceQuery.Description
	resource.Category = resourceQuery.Category
	resource.ContentCategory = resourceQuery.ContentCategory
	resource.OwnerId = &resourceQuery.OwnerId

	if err := tx.Save(resource).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	return &resource, tx.Commit().Error
}

func (ctx *MahresourcesContext) AddResource(file File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	fileMime, err := mimetype.DetectReader(file)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		tx.Rollback()
		return nil, err
	}

	fileBytes, err := ioutil.ReadAll(file)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	h := sha1.New()
	h.Write(fileBytes)
	hash := hex.EncodeToString(h.Sum(nil))
	folder := "/resources/" + hash[0:2] + "/" + hash[2:4] + "/" + hash[4:6] + "/"

	if err := ctx.fs.MkdirAll(folder, 0777); err != nil {
		tx.Rollback()
		return nil, err
	}

	filePath := path.Join(folder, hash+fileMime.Extension())
	stat, statError := ctx.fs.Stat(filePath)

	if statError == nil && stat != nil {
		tx.Rollback()
		return nil, errors.New("file already exists")
	}

	savedFile, err := ctx.fs.Create(filePath)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	defer func(savedFile afero.File) { _ = savedFile.Close() }(savedFile)

	if _, err := savedFile.Write(fileBytes); err != nil {
		tx.Rollback()
		return nil, err
	}

	name := fileName

	if resourceQuery.Name != "" {
		name = resourceQuery.Name
	}

	if resourceQuery.Meta == "" {
		resourceQuery.Meta = "{}"
	}

	res := &models.Resource{
		Name:            name,
		Hash:            hash,
		HashType:        "SHA1",
		Location:        filePath,
		Meta:            []byte(resourceQuery.Meta),
		Category:        resourceQuery.Category,
		ContentType:     fileMime.String(),
		ContentCategory: resourceQuery.ContentCategory,
		FileSize:        int64(len(fileBytes)) << 3,
		OwnerId:         &resourceQuery.OwnerId,
	}

	if err := tx.Save(res).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if len(resourceQuery.Groups) > 0 {
		groups := make([]models.Group, len(resourceQuery.Groups))
		for i, v := range resourceQuery.Groups {
			groups[i] = models.Group{
				ID: v,
			}
		}

		if createGroupsErr := tx.Model(&res).Association("Groups").Append(&groups); createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	if len(resourceQuery.Notes) > 0 {
		notes := make([]models.Note, len(resourceQuery.Notes))
		for i, v := range resourceQuery.Notes {
			notes[i] = models.Note{
				ID: v,
			}
		}

		if createNotesErr := tx.Model(&res).Association("Notes").Append(&notes); createNotesErr != nil {
			tx.Rollback()
			return nil, createNotesErr
		}
	}

	if len(resourceQuery.Tags) > 0 {
		tags := make([]models.Tag, len(resourceQuery.Tags))
		for i, v := range resourceQuery.Tags {
			tags[i] = models.Tag{
				ID: v,
			}
		}

		if createTagsErr := tx.Model(&res).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return nil, createTagsErr
		}
	}

	return res, tx.Commit().Error
}

func (ctx *MahresourcesContext) LoadOrCreateThumbnailForResource(resourceId, width, height uint) (*models.Preview, error) {
	var existingThumbnail models.Preview
	ctx.db.Where(&models.Preview{Width: width, Height: height, ResourceId: &resourceId}).First(&existingThumbnail)

	if existingThumbnail.ID != 0 {
		return &existingThumbnail, nil
	}

	resource, err := ctx.GetResource(resourceId)

	fs, storageError := ctx.getFsForStorageLocation(resource.StorageLocation)

	if storageError != nil {
		return nil, storageError
	}

	if err != nil {
		return nil, err
	}

	var newImage image.Image

	if strings.HasPrefix(resource.ContentType, "image/") {
		file, err := fs.Open(resource.Location)

		if err != nil {
			return nil, err
		}

		originalImage, _, err := image.Decode(file)
		newImage = resize.Resize(width, height, originalImage, resize.Lanczos3)
	} else if strings.HasPrefix(resource.ContentType, "video/") {
		return nil, nil
	}

	if newImage == nil {
		return nil, nil
	}

	var buf bytes.Buffer
	err = jpeg.Encode(&buf, newImage, nil)
	fileBytes, err := ioutil.ReadAll(&buf)

	if err != nil {
		return nil, err
	}

	preview := &models.Preview{
		Data:        fileBytes,
		Width:       width,
		Height:      height,
		ContentType: "image/jpeg",
		Resource:    resource,
		ResourceId:  &resource.ID,
	}

	ctx.db.Save(preview)

	return preview, nil
}

func (ctx *MahresourcesContext) getFsForStorageLocation(storageLocation *string) (afero.Fs, error) {
	if storageLocation != nil {
		altFs, ok := ctx.altFileSystems[*storageLocation]

		if !ok {
			return nil, errors.New(fmt.Sprintf("alt fs '%v' is not attached", *storageLocation))
		}

		return altFs, nil
	}

	return ctx.fs, nil
}

func (ctx *MahresourcesContext) DeleteResource(resourceId uint) error {
	resource := models.Resource{ID: resourceId}

	if err := ctx.db.Model(&resource).First(&resource).Error; err != nil {
		return err
	}

	fs, storageError := ctx.getFsForStorageLocation(resource.StorageLocation)

	if storageError != nil {
		return storageError
	}

	if err := fs.Remove(resource.Location); err != nil {
		return err
	}

	return ctx.db.Select(clause.Associations).Delete(&resource).Error
}

func (ctx *MahresourcesContext) ResourceMetaKeys() (*[]fieldResult, error) {
	return metaKeys(ctx, "resources")
}
