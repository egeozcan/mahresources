package application_context

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
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
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"math"
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

	return &resource, ctx.db.Preload(clause.Associations, pageLimit).First(&resource, id).Error
}

func (ctx *MahresourcesContext) GetSimilarResources(id uint) (*[]*models.Resource, error) {
	var resources []*models.Resource

	hashQuery := ctx.db.Table("image_hashes rootHash").
		Select("d_hash").
		Where("rootHash.resource_id = ?", id).
		Limit(1)

	sameHashIdsQuery := ctx.db.Table("image_hashes").
		Select("resource_id").
		Group("resource_id").
		Where("d_hash = (?)", hashQuery)

	return &resources, ctx.db.
		Preload("Tags").
		Joins("Owner").
		Where("resources.id IN (?)", sameHashIdsQuery).
		Where("resources.id <> ?", id).
		Find(&resources).Error
}

func (ctx *MahresourcesContext) GetResourceCount(query *query_models.ResourceSearchQuery) (int64, error) {
	var resource models.Resource
	var count int64

	return count, ctx.db.Scopes(database_scopes.ResourceQuery(query, true, ctx.db)).Model(&resource).Count(&count).Error
}

func (ctx *MahresourcesContext) GetResources(offset, maxResults int, query *query_models.ResourceSearchQuery) (*[]models.Resource, error) {
	var resources []models.Resource
	resLimit := maxResults

	if query.MaxResults > 0 {
		resLimit = int(query.MaxResults)
	}

	return &resources, ctx.db.Scopes(database_scopes.ResourceQuery(query, false, ctx.db)).
		Limit(resLimit).
		Offset(offset).
		Preload("Tags").
		Preload("Owner").
		Find(&resources).
		Error
}

func (ctx *MahresourcesContext) GetResourcesWithIds(ids *[]uint) (*[]*models.Resource, error) {
	var resources []*models.Resource

	if len(*ids) == 0 {
		return &resources, nil
	}

	return &resources, ctx.db.Find(&resources, ids).Preload("Tags").Error
}

func (ctx *MahresourcesContext) EditResource(resourceQuery *query_models.ResourceEditor) (*models.Resource, error) {
	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var resource models.Resource

	if err := tx.Preload(clause.Associations, pageLimit).First(&resource, resourceQuery.ID).Error; err != nil {
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
	urls := strings.Split(resourceQuery.URL, "\n")
	var firstResource *models.Resource
	var firstError error

	setError := func(err error) {
		if firstError == nil {
			firstError = err
		}
		print(err)
	}

	for _, url := range urls {
		(func(url string) {
			resp, err := http.Get(url)

			if err != nil {
				setError(err)
				return
			}

			defer resp.Body.Close()

			if resourceQuery.GroupName != "" {
				category := models.Category{Name: resourceQuery.GroupCategoryName}

				if resourceQuery.GroupCategoryName != "" {
					if err := ctx.db.Where(&category).First(&category).Error; err != nil {
						if err := ctx.db.Save(&category).Error; err != nil {
							setError(err)
							return
						}
					}
				}

				group := models.Group{CategoryId: &category.ID, Name: resourceQuery.GroupName}

				if err := ctx.db.Where(&group).First(&group).Error; err != nil {
					group.Meta = []byte(resourceQuery.GroupMeta)
					if err := ctx.db.Save(&group).Error; err != nil {
						setError(err)
						return
					}
				}

				resourceQuery.OwnerId = group.ID
			}

			res, err := ctx.AddResource(resp.Body, resourceQuery.FileName, &query_models.ResourceCreator{
				ResourceQueryBase: query_models.ResourceQueryBase{
					Name:             resourceQuery.Name,
					Description:      resourceQuery.Description,
					OwnerId:          resourceQuery.OwnerId,
					Groups:           resourceQuery.Groups,
					Tags:             resourceQuery.Tags,
					Notes:            resourceQuery.Notes,
					Meta:             resourceQuery.Meta,
					ContentCategory:  resourceQuery.ContentCategory,
					Category:         resourceQuery.Category,
					OriginalName:     url,
					OriginalLocation: url,
				},
			})

			if firstResource == nil {
				firstResource = res
			}

			if err != nil {
				setError(err)
				return
			}
		})(strings.TrimSpace(url))
	}

	if firstResource == nil {
		return nil, firstError
	}

	return firstResource, nil
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

	fs, err := ctx.GetFsForStorageLocation(&resourceQuery.PathName)

	if err != nil {
		return nil, err
	}

	file, err := fs.Open(resourceQuery.LocalPath)

	if err != nil {
		return nil, err
	}

	defer file.Close()

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
		FileSize:         int64(len(fileBytes)),
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

	var existingResource models.Resource

	if existingNotFoundErr := tx.Where("hash = ?", hash).Preload("Groups").First(&existingResource).Error; existingNotFoundErr == nil {
		if resourceQuery.OwnerId == *existingResource.OwnerId {
			if len(resourceQuery.Groups) > 0 {
				go func() {
					groups, _ := ctx.GetGroupsWithIds(&resourceQuery.Groups)
					_ = ctx.db.Model(&existingResource).Association("Groups").Append(groups)
				}()
			}
			tx.Rollback()
			return nil, errors.New(fmt.Sprintf("existing resource (%v) with same parent", existingResource.ID))
		}

		for _, group := range existingResource.Groups {
			if resourceQuery.OwnerId == group.ID {
				tx.Rollback()
				return nil, errors.New(fmt.Sprintf("existing resource (%v) with same relation", existingResource.ID))
			}
		}

		groups := &[]*models.Group{
			{ID: resourceQuery.OwnerId},
		}

		if attachToGroupErr := tx.Model(&existingResource).Association("Groups").Append(groups); attachToGroupErr != nil {
			tx.Rollback()
			return nil, attachToGroupErr
		}

		return &existingResource, tx.Commit().Error
	}

	folder := "/resources/" + hash[0:2] + "/" + hash[2:4] + "/" + hash[4:6] + "/"

	if err := ctx.fs.MkdirAll(folder, 0777); err != nil {
		tx.Rollback()
		return nil, err
	}

	var savedFile afero.File
	fileExists := false

	filePath := path.Join(folder, hash+fileMime.Extension())
	stat, statError := ctx.fs.Stat(filePath)

	if statError == nil && stat != nil {
		savedFile, err = ctx.fs.Open(filePath)
		println("reusing stale file at " + filePath)
		fileExists = true
	} else {
		savedFile, err = ctx.fs.Create(filePath)
	}

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	defer func(savedFile afero.File) { _ = savedFile.Close() }(savedFile)

	if !fileExists {
		if _, err := savedFile.Write(fileBytes); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	name := fileName

	if resourceQuery.OriginalName == "" {
		resourceQuery.OriginalName = fileName
	}

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
		FileSize:         int64(len(fileBytes)),
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
	var nullThumbnail *models.Preview
	var fileBytes []byte

	width = uint(math.Min(constants.MaxThumbWidth, float64(width)))
	height = uint(math.Min(constants.MaxThumbHeight, float64(height)))

	if err := ctx.db.Where(&models.Preview{Width: width, Height: height, ResourceId: &resourceId}).Omit(clause.Associations).First(&existingThumbnail).Error; err == nil {
		return &existingThumbnail, nil
	}

	var resource models.Resource

	if err := ctx.db.Omit(clause.Associations).First(&resource, resourceId).Error; err != nil {
		return nil, err
	}

	fs, storageError := ctx.GetFsForStorageLocation(resource.StorageLocation)

	if storageError != nil {
		return nil, storageError
	}

	if err := ctx.db.Where(&models.Preview{Width: 0, Height: 0, ResourceId: &resourceId}).Omit(clause.Associations).First(nullThumbnail).Error; err != nil {
		name := resource.GetCleanLocation() + constants.ThumbFileSuffix
		println("will try opening", name)

		if file, fopenErr := fs.Open(name); fopenErr == nil && file != nil {
			defer file.Close()

			fileBytes, err = ioutil.ReadAll(file)

			if err == nil {
				nullThumbnail = &models.Preview{
					Data:        fileBytes,
					Width:       0,
					Height:      0,
					ContentType: "image/jpeg",
					ResourceId:  &resource.ID,
				}

				ctx.db.Save(nullThumbnail)
			}
		} else {
			fmt.Println("ok", file, fopenErr)
		}
	}

	if nullThumbnail != nil {
		var newImage image.Image

		originalImage, _, err := image.Decode(bytes.NewReader((*nullThumbnail).Data))

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

	} else if strings.HasPrefix(resource.ContentType, "image/") {
		file, err := fs.Open(resource.GetCleanLocation())

		if err != nil {
			return nil, err
		}

		defer file.Close()

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
		file, err := fs.Open(resource.GetCleanLocation())

		if err != nil {
			return nil, err
		}

		defer file.Close()

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

	cmd := exec.Command(ctx.Config.FfmpegPath,
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

func (ctx *MahresourcesContext) GetFsForStorageLocation(storageLocation *string) (afero.Fs, error) {
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

	fs, storageErr := ctx.GetFsForStorageLocation(resource.StorageLocation)

	if storageErr != nil {
		return storageErr
	}

	subFolder := "deleted"

	if resource.StorageLocation != nil && *resource.StorageLocation != "" {
		subFolder = *resource.StorageLocation
	}

	folder := fmt.Sprintf("/deleted/%v/", subFolder)

	if err := ctx.fs.MkdirAll(folder, 0777); err != nil {
		return err
	}

	filePath := path.Join(folder, fmt.Sprintf("%v__%v__%v___%v", resource.Hash, resource.ID, *resource.OwnerId, strings.ReplaceAll(path.Clean(path.Base(resource.GetCleanLocation())), "\\", "_")))

	file, openErr := fs.Open(resource.GetCleanLocation())

	if openErr == nil {
		backup, createErr := ctx.fs.Create(filePath)

		if createErr != nil {
			_ = file.Close()
			return createErr
		}

		defer backup.Close()

		_, copyErr := io.Copy(backup, file)

		if copyErr != nil {
			_ = file.Close()
			return copyErr
		}

		_ = file.Close()
	}

	if err := ctx.db.Select(clause.Associations).Delete(&resource).Error; err != nil {
		return err
	}

	_ = fs.Remove(resource.GetCleanLocation())

	return nil
}

func (ctx *MahresourcesContext) ResourceMetaKeys() (*[]fieldResult, error) {
	return metaKeys(ctx, "resources")
}

func (ctx *MahresourcesContext) BulkRemoveTagsFromResources(query *query_models.BulkEditQuery) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for _, editedId := range query.EditedId {
			tag, err := ctx.GetTag(editedId)

			if err != nil {
				return err
			}

			for _, id := range query.ID {
				appendErr := tx.Model(&models.Resource{ID: id}).Association("Tags").Delete(tag)

				if appendErr != nil {
					return appendErr
				}
			}
		}

		return nil
	})
}

func (ctx *MahresourcesContext) BulkReplaceTagsFromResources(query *query_models.BulkEditQuery) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		tags := make([]*models.Tag, len(query.EditedId))

		for i, editedId := range query.EditedId {
			tag, err := ctx.GetTag(editedId)

			if err != nil {
				return err
			}

			tags[i] = tag
		}

		for _, id := range query.ID {
			appendErr := tx.Model(&models.Resource{ID: id}).Association("Tags").Replace(tags)

			if appendErr != nil {
				return appendErr
			}
		}

		return nil
	})
}

func (ctx *MahresourcesContext) BulkAddMetaToResources(query *query_models.BulkEditMetaQuery) error {
	var resource models.Resource

	return ctx.db.
		Model(&resource).
		Where("id in ?", query.ID).
		Update("Meta", gorm.Expr("Meta || ?", query.Meta)).Error
}

func (ctx *MahresourcesContext) BulkAddTagsToResources(query *query_models.BulkEditQuery) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for _, editedId := range query.EditedId {
			tag, err := ctx.GetTag(editedId)

			if err != nil {
				return err
			}

			for _, id := range query.ID {
				appendErr := tx.Model(&models.Resource{ID: id}).Association("Tags").Append(tag)

				if appendErr != nil {
					return appendErr
				}
			}
		}

		return nil
	})
}

func (ctx *MahresourcesContext) BulkAddGroupsToResources(query *query_models.BulkEditQuery) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for _, editedId := range query.EditedId {
			group, err := ctx.GetGroup(editedId)

			if err != nil {
				return err
			}

			for _, id := range query.ID {
				appendErr := tx.Model(&models.Resource{ID: id}).Association("Groups").Append(group)

				if appendErr != nil {
					return appendErr
				}
			}
		}

		return nil
	})
}

func (ctx *MahresourcesContext) BulkDeleteResources(query *query_models.BulkQuery) error {
	for _, id := range query.ID {
		if err := ctx.DeleteResource(id); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *MahresourcesContext) GetPopularResourceTags() ([]struct {
	Name  string
	Id    uint
	count int
}, error) {
	var res []struct {
		Name  string
		Id    uint
		count int
	}

	return res, ctx.db.
		Table("resource_tags").
		Select("t.id AS Id, t.name AS name, count(*) AS count").
		Joins("INNER JOIN tags t ON t.id = resource_tags.tag_id").
		Group("t.id, t.name").
		Order("count(*) DESC").
		Limit(20).
		Scan(&res).
		Error
}

func (ctx *MahresourcesContext) MergeResources(winnerId uint, loserIds []uint) error {
	if len(loserIds) == 0 || winnerId == 0 {
		return errors.New("incorrect parameters")
	}

	for i, id := range loserIds {
		if id == 0 {
			return errors.New(fmt.Sprintf("loser number %v has 0 id", i+1))
		}

		if id == winnerId {
			return errors.New("winner cannot be one of the losers")
		}
	}

	return ctx.WithTransaction(func(transactionCtx *MahresourcesContext) error {
		var losers []*models.Resource

		tx := transactionCtx.db

		if loadResourcesErr := tx.Preload(clause.Associations).Find(&losers, &loserIds).Error; loadResourcesErr != nil {
			return loadResourcesErr
		}

		if winnerId == 0 || loserIds == nil || len(loserIds) == 0 {
			return nil
		}

		var winner models.Resource

		if err := tx.Preload(clause.Associations).First(&winner, winnerId).Error; err != nil {
			return err
		}

		deletedResBackups := make(map[string]types.JSON)

		for _, loser := range losers {

			for _, tag := range loser.Tags {
				if err := tx.Exec(`INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, tag.ID).Error; err != nil {
					return err
				}
			}
			for _, note := range loser.Notes {
				if err := tx.Exec(`INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, note.ID).Error; err != nil {
					return err
				}
			}
			for _, group := range loser.Groups {
				if err := tx.Exec(`INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, group.ID).Error; err != nil {
					return err
				}
			}
			if err := tx.Exec(`INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, loser.OwnerId).Error; err != nil {
				return err
			}

			backupData, err := json.Marshal(loser)

			if err != nil {
				return err
			}

			deletedResBackups[fmt.Sprintf("resource_%v", loser.ID)] = backupData
			fmt.Printf("%#v\n", deletedResBackups)

			switch transactionCtx.Config.DbType {
			case constants.DbTypePosgres:
				err = tx.Exec(`
				UPDATE resources
				SET meta = coalesce((SELECT meta FROM resources WHERE id = ?), '{}'::jsonb) || meta
				WHERE id = ?
			`, loser.ID, winnerId).Error
			case constants.DbTypeSqlite:
				err = tx.Exec(`
				UPDATE resources
				SET meta = json_patch(meta, coalesce((SELECT meta FROM resources WHERE id = ?), '{}'))
				WHERE id = ?
			`, loser.ID, winnerId).Error
			default:
				err = errors.New("db doesn't support merging meta")
			}

			if err != nil {
				return err
			}

			err = transactionCtx.DeleteResource(loser.ID)

			if err != nil {
				return err
			}
		}

		fmt.Printf("%#v\n", deletedResBackups)

		backupObj := make(map[string]any)
		backupObj["backups"] = deletedResBackups

		backups, err := json.Marshal(&backupObj)

		if err != nil {
			return err
		}

		fmt.Println(string(backups))

		if transactionCtx.Config.DbType == constants.DbTypePosgres {
			if err := tx.Exec("update resources set meta = meta || ? where id = ?", backups, winner.ID).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
