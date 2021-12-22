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
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"net/http"
	"os"
	"os/exec"
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

func (ctx *MahresourcesContext) GetResourceCount(query *query_models.ResourceSearchQuery) (int64, error) {
	var resource models.Resource
	var count int64

	return count, ctx.db.Scopes(database_scopes.ResourceQuery(query, true)).Model(&resource).Count(&count).Error
}

func (ctx *MahresourcesContext) GetResources(offset, maxResults int, query *query_models.ResourceSearchQuery) (*[]models.Resource, error) {
	var resources []models.Resource
	resLimit := maxResults

	if query.MaxResults > 0 {
		resLimit = int(query.MaxResults)
	}

	return &resources, ctx.db.Scopes(database_scopes.ResourceQuery(query, false)).Limit(resLimit).Offset(offset).Preload("Tags").Find(&resources).Error
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
	resource.OriginalName = resourceQuery.OriginalName
	resource.OriginalLocation = resourceQuery.OriginalLocation
	resource.Category = resourceQuery.Category
	resource.ContentCategory = resourceQuery.ContentCategory
	resource.OwnerId = &resourceQuery.OwnerId

	if err := tx.Save(resource).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	return &resource, tx.Commit().Error
}

func (ctx *MahresourcesContext) AddRemoteResource(resourceQuery *query_models.ResourceFromRemoteCreator) (*models.Resource, error) {
	resp, err := http.Get(resourceQuery.URL)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resourceQuery.GroupName != "" {
		category := models.Category{Name: resourceQuery.GroupCategoryName}

		if resourceQuery.GroupCategoryName != "" {
			if err := ctx.db.Where(&category).First(&category).Error; err != nil {
				if err := ctx.db.Save(&category).Error; err != nil {
					return nil, err
				}
			}
		}

		group := models.Group{CategoryId: &category.ID, Name: resourceQuery.GroupName}

		if err := ctx.db.Where(&group).First(&group).Error; err != nil {
			group.Meta = []byte(resourceQuery.GroupMeta)
			if err := ctx.db.Save(&group).Error; err != nil {
				return nil, err
			}
		}

		resourceQuery.OwnerId = group.ID
	}

	return ctx.AddResource(resp.Body, resourceQuery.FileName, &query_models.ResourceCreator{
		ResourceQueryBase: resourceQuery.ResourceQueryBase,
	})
}

func (ctx *MahresourcesContext) AddLocalResource(fileName string, resourceQuery *query_models.ResourceFromLocalCreator) (*models.Resource, error) {
	var existingResource models.Resource

	query := ctx.db.Where("location = ? AND storage_location = ?", resourceQuery.LocalPath, resourceQuery.PathName).First(&existingResource)
	if err := query.Error; err == nil && existingResource.ID != 0 {
		fmt.Println(fmt.Sprintf("we already have %v, moving on", resourceQuery.LocalPath))
		// this resource is already saved, return it instead
		return &existingResource, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// some other db problem. record not found would have been ok, as we actually expect it to be the case.
		// here something else went wrong
		return nil, err
	}

	fs, err := ctx.getFsForStorageLocation(&resourceQuery.PathName)

	if err != nil {
		return nil, err
	}

	file, err := fs.Open(resourceQuery.LocalPath)

	if err != nil {
		return nil, err
	}

	fileMime, err := mimetype.DetectReader(file)

	if err != nil {
		return nil, err
	}

	fileBytes, err := ioutil.ReadAll(file)

	if err != nil {
		return nil, err
	}

	h := sha1.New()
	h.Write(fileBytes)
	hash := hex.EncodeToString(h.Sum(nil))

	res := &models.Resource{
		Name:             fileName,
		Hash:             hash,
		HashType:         "SHA1",
		Location:         resourceQuery.LocalPath,
		Meta:             []byte(resourceQuery.Meta),
		Category:         resourceQuery.Category,
		ContentType:      fileMime.String(),
		ContentCategory:  resourceQuery.ContentCategory,
		FileSize:         int64(len(fileBytes)) << 3,
		OwnerId:          &resourceQuery.OwnerId,
		StorageLocation:  &resourceQuery.PathName,
		Description:      resourceQuery.Description,
		OriginalLocation: resourceQuery.OriginalLocation,
		OriginalName:     resourceQuery.OriginalName,
	}

	if err := ctx.db.Save(res).Error; err != nil {
		return nil, err
	}

	return res, nil
}

func (ctx *MahresourcesContext) AddResource(file File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	fileBytes, err := ioutil.ReadAll(file)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	fileMime, err := mimetype.DetectReader(bytes.NewBuffer(fileBytes))

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
		Name:             name,
		Hash:             hash,
		HashType:         "SHA1",
		Location:         filePath,
		Meta:             []byte(resourceQuery.Meta),
		Category:         resourceQuery.Category,
		ContentType:      fileMime.String(),
		ContentCategory:  resourceQuery.ContentCategory,
		FileSize:         int64(len(fileBytes)) << 3,
		OwnerId:          &resourceQuery.OwnerId,
		Description:      resourceQuery.Description,
		OriginalLocation: resourceQuery.OriginalLocation,
		OriginalName:     resourceQuery.OriginalName,
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
	var fileBytes []byte

	if err := ctx.db.Where(&models.Preview{Width: width, Height: height, ResourceId: &resourceId}).First(&existingThumbnail).Error; err == nil {
		return &existingThumbnail, nil
	}

	resource, err := ctx.GetResource(resourceId)

	if err != nil {
		return nil, err
	}

	fs, storageError := ctx.getFsForStorageLocation(resource.StorageLocation)

	if storageError != nil {
		return nil, storageError
	}

	if strings.HasPrefix(resource.ContentType, "image/") {
		file, err := fs.Open(resource.Location)

		if err != nil {
			return nil, err
		}

		var newImage image.Image

		originalImage, _, err := image.Decode(file)

		if err != nil {
			return nil, err
		}

		newImage = resize.Resize(width, height, originalImage, resize.Lanczos3)

		var buf bytes.Buffer

		if err := jpeg.Encode(&buf, newImage, nil); err != nil {
			return nil, err
		}

		fileBytes, err = ioutil.ReadAll(&buf)

		if err != nil {
			return nil, err
		}

	} else if strings.HasPrefix(resource.ContentType, "video/") {
		file, err := fs.Open(resource.Location)

		if err != nil {
			return nil, err
		}

		var newImage image.Image

		resultBuffer := bytes.NewBuffer(make([]byte, 0))
		sectionReader := io.NewSectionReader(file, 0, 5000000)

		if err := ctx.createThumbFromVideo(sectionReader, resultBuffer); err != nil {
			return nil, err
		}

		originalImage, _, err := image.Decode(resultBuffer)

		if err != nil {
			return nil, err
		}

		newImage = resize.Resize(width, height, originalImage, resize.Lanczos3)

		var buf bytes.Buffer

		if err := jpeg.Encode(&buf, newImage, nil); err != nil {
			return nil, err
		}

		fileBytes, err = ioutil.ReadAll(&buf)

		if err != nil {
			return nil, err
		}

	} else {
		return nil, nil
	}

	preview := &models.Preview{
		Data:        fileBytes,
		Width:       width,
		Height:      height,
		ContentType: "image/jpeg",
		ResourceId:  &resource.ID,
	}

	ctx.db.Save(preview)

	return preview, nil
}

func (ctx *MahresourcesContext) createThumbFromVideo(file io.Reader, resultBuffer *bytes.Buffer) error {
	var buffer []byte

	if buf, err := ioutil.ReadAll(file); err != nil {
		return err
	} else {
		buffer = buf
	}

	cmd := exec.Command(ctx.config.FfmpegPath,
		"-i", "-", // stdin
		"-ss", "00:00:0",
		"-vframes", "1",
		"-c:v", "png",
		"-f", "image2pipe",
		"-",
	)

	cmd.Stderr = os.Stderr
	cmd.Stdout = resultBuffer

	var stdin io.WriteCloser

	if stdinAtt, err := cmd.StdinPipe(); err != nil {
		return err
	} else {
		stdin = stdinAtt
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	_, _ = stdin.Write(buffer)

	if err := stdin.Close(); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
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
