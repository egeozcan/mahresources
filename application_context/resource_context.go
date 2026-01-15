package application_context

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/server/interfaces"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/anthonynsimon/bild/transform"

	"github.com/gabriel-vasile/mimetype"
	"github.com/nfnt/resize"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
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
	resource.Owner = &models.Group{ID: resourceQuery.OwnerId}

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

			name := resourceQuery.FileName

			// if the name is an empty string, try to get the name from the URL
			if name == "" {
				name = path.Base(url)
			}

			res, err := ctx.AddResource(resp.Body, resourceQuery.FileName, &query_models.ResourceCreator{
				ResourceQueryBase: query_models.ResourceQueryBase{
					Name:             name,
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

	fileBytes, err := io.ReadAll(file)

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

func (ctx *MahresourcesContext) AddResource(file interfaces.File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	tempFile, err := os.CreateTemp("", "upload-")
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	defer os.Remove(tempFile.Name())

	// Copy the contents of the uploaded file to the temporary file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	fileMime, err := mimetype.DetectFile(tempFile.Name())
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Calculate the SHA1 hash of the uploaded file
	h := sha1.New()
	_, err = io.Copy(h, tempFile)

	_, err = tempFile.Seek(0, io.SeekStart)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

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
		_, err = io.Copy(savedFile, tempFile)
		if err != nil {
			tx.Rollback()
			return nil, err
		}

		_, err = tempFile.Seek(0, io.SeekStart)
		if err != nil {
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

	width := 0
	height := 0

	// if it's an image, add the width and height to the meta
	if strings.HasPrefix(fileMime.String(), "image/") {
		img, _, err := image.Decode(tempFile)
		if err == nil {
			bounds := img.Bounds()
			width = bounds.Max.X
			height = bounds.Max.Y
		}
	}

	fileInfo, err := tempFile.Stat()
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	fileSize := fileInfo.Size()

	res := &models.Resource{
		Name:             name,
		Hash:             hash,
		HashType:         "SHA1",
		Location:         filePath,
		Meta:             []byte(resourceQuery.Meta),
		Category:         resourceQuery.Category,
		ContentType:      fileMime.String(),
		ContentCategory:  resourceQuery.ContentCategory,
		FileSize:         fileSize,
		OwnerId:          &resourceQuery.OwnerId,
		Description:      resourceQuery.Description,
		OriginalLocation: resourceQuery.OriginalLocation,
		OriginalName:     resourceQuery.OriginalName,
		Width:            uint(width),
		Height:           uint(height),
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

// LoadOrCreateThumbnailForResource generates or retrieves a thumbnail for a given resource.
// It respects the provided context for cancellation and timeout.
func (ctx *MahresourcesContext) LoadOrCreateThumbnailForResource(
	resourceId, width, height uint,
	httpContext context.Context,
) (*models.Preview, error) {
	// Acquire the ThumbnailGenerationLock for the given resourceId with context support
	if err := ctx.locks.ThumbnailGenerationLock.AcquireContext(httpContext, resourceId); err != nil {
		return nil, fmt.Errorf("failed to acquire thumbnail generation lock: %w", err)
	}
	defer ctx.locks.ThumbnailGenerationLock.Release(resourceId)

	// Ensure width and height do not exceed maximum allowed values
	width = uint(math.Min(constants.MaxThumbWidth, float64(width)))
	height = uint(math.Min(constants.MaxThumbHeight, float64(height)))

	// Attempt to retrieve an existing thumbnail with the specified dimensions
	existingThumbnail, err := ctx.getExistingThumbnail(resourceId, width, height, httpContext)
	if err == nil {
		return &existingThumbnail, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("error retrieving existing thumbnail: %w", err)
	}

	// Retrieve the resource from the database
	resource, err := ctx.getResource(resourceId, httpContext)
	if err != nil {
		return nil, fmt.Errorf("error retrieving resource: %w", err)
	}

	// Get the filesystem interface for the resource's storage location
	fs, storageError := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if storageError != nil {
		return nil, fmt.Errorf("error getting filesystem: %w", storageError)
	}

	// Attempt to find or create a null thumbnail (original image without size)
	nullThumbnail, fileBytes, err := ctx.getOrCreateNullThumbnail(resource, fs, httpContext)
	if err != nil {
		return nil, fmt.Errorf("error handling null thumbnail: %w", err)
	}

	// Depending on the resource's content type, generate the appropriate thumbnail
	if nullThumbnail.ID != 0 {
		// If a null thumbnail exists, resize it to the desired dimensions
		fileBytes, err = ctx.generateImageThumbnail(nullThumbnail.Data, width, height)
		if err != nil {
			return nil, fmt.Errorf("error generating image thumbnail from null thumbnail: %w", err)
		}
	} else if strings.HasPrefix(resource.ContentType, "image/") {
		// Handle image resources by generating a thumbnail directly from the image file
		fileBytes, err = ctx.generateImageThumbnailFromFile(fs, resource.GetCleanLocation(), width, height, httpContext)
		if err != nil {
			return nil, fmt.Errorf("error generating image thumbnail from file: %w", err)
		}
	} else if strings.HasPrefix(resource.ContentType, "video/") {
		// Handle video resources by generating a thumbnail from the video
		fileBytes, err = ctx.generateVideoThumbnail(resource, fs, width, height, httpContext)
		if err != nil {
			return nil, fmt.Errorf("error generating video thumbnail: %w", err)
		}
	} else {
		// Unsupported content type; no thumbnail to generate
		return nil, nil
	}

	// Create and save the new preview (thumbnail) to the database
	preview := &models.Preview{
		Data:        fileBytes,
		Width:       width,
		Height:      height,
		ContentType: "image/jpeg",
		ResourceId:  &resource.ID,
	}

	if err := ctx.db.WithContext(httpContext).Save(preview).Error; err != nil {
		return nil, fmt.Errorf("error saving new thumbnail to database: %w", err)
	}

	return preview, nil
}

////////////////////////////////////////////////////////////////////////////////
// Helper Functions
////////////////////////////////////////////////////////////////////////////////

// getExistingThumbnail retrieves an existing thumbnail with the specified dimensions.
func (ctx *MahresourcesContext) getExistingThumbnail(
	resourceId, width, height uint,
	httpContext context.Context,
) (models.Preview, error) {
	var thumbnail models.Preview
	err := ctx.db.WithContext(httpContext).
		Where(&models.Preview{
			Width:      width,
			Height:     height,
			ResourceId: &resourceId,
		}).
		Omit(clause.Associations).
		First(&thumbnail).Error
	return thumbnail, err
}

// getResource retrieves the resource from the database.
func (ctx *MahresourcesContext) getResource(
	resourceId uint,
	httpContext context.Context,
) (models.Resource, error) {
	var resource models.Resource
	err := ctx.db.WithContext(httpContext).
		Omit(clause.Associations).
		First(&resource, resourceId).Error
	return resource, err
}

// getOrCreateNullThumbnail attempts to retrieve a null thumbnail or create one if it doesn't exist.
func (ctx *MahresourcesContext) getOrCreateNullThumbnail(
	resource models.Resource,
	fs afero.Fs,
	httpContext context.Context,
) (models.Preview, []byte, error) {
	var nullThumbnail models.Preview
	var fileBytes []byte

	err := ctx.db.WithContext(httpContext).
		Where(&models.Preview{
			Width:      0,
			Height:     0,
			ResourceId: &resource.ID,
		}).
		Omit(clause.Associations).
		First(&nullThumbnail).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Null thumbnail doesn't exist; attempt to create it from the original image
		name := resource.GetCleanLocation() + constants.ThumbFileSuffix
		fmt.Println("Attempting to open", name)

		file, fopenErr := fs.Open(name)
		if fopenErr != nil {
			fmt.Println("Failed to open file:", fopenErr)
			return nullThumbnail, fileBytes, nil // Return empty fileBytes; no null thumbnail
		}
		defer file.Close()

		// Read the file bytes
		fileBytes, readErr := io.ReadAll(file)
		if readErr != nil {
			return nullThumbnail, fileBytes, fmt.Errorf("failed to read null thumbnail file: %w", readErr)
		}

		// Initialize nullThumbnail correctly
		nullThumbnail = models.Preview{
			Data:        fileBytes,
			Width:       0,
			Height:      0,
			ContentType: "image/jpeg",
			ResourceId:  &resource.ID,
		}

		// Save the null thumbnail to the database
		if saveErr := ctx.db.WithContext(httpContext).Save(&nullThumbnail).Error; saveErr != nil {
			return nullThumbnail, fileBytes, fmt.Errorf("failed to save null thumbnail to database: %w", saveErr)
		}
	} else if err != nil {
		// An unexpected error occurred while retrieving the null thumbnail
		return nullThumbnail, fileBytes, fmt.Errorf("error retrieving null thumbnail: %w", err)
	}

	return nullThumbnail, fileBytes, nil
}

// generateImageThumbnail resizes the provided image data to the desired dimensions.
func (ctx *MahresourcesContext) generateImageThumbnail(
	imageData []byte,
	width, height uint,
) ([]byte, error) {
	originalImage, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image data: %w", err)
	}

	// Resize the image to desired dimensions
	newImage := resize.Resize(width, height, originalImage, resize.Lanczos3)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, newImage, nil); err != nil {
		return nil, fmt.Errorf("failed to encode resized image: %w", err)
	}

	// Read the resized image bytes
	fileBytes, err := io.ReadAll(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read resized image bytes: %w", err)
	}

	return fileBytes, nil
}

// generateImageThumbnailFromFile generates a thumbnail from the image file at the given location.
func (ctx *MahresourcesContext) generateImageThumbnailFromFile(
	fs afero.Fs,
	location string,
	width, height uint,
	httpContext context.Context,
) ([]byte, error) {
	file, err := fs.Open(location)
	if err != nil {
		return nil, fmt.Errorf("failed to open image file: %w", err)
	}
	defer file.Close()

	originalImage, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image file: %w", err)
	}

	// Resize the image to desired dimensions
	newImage := resize.Resize(width, height, originalImage, resize.Lanczos3)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, newImage, nil); err != nil {
		return nil, fmt.Errorf("failed to encode resized image: %w", err)
	}

	// Read the resized image bytes
	fileBytes, err := io.ReadAll(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read resized image bytes: %w", err)
	}

	return fileBytes, nil
}

// generateVideoThumbnail generates a thumbnail from the video resource.
func (ctx *MahresourcesContext) generateVideoThumbnail(
	resource models.Resource,
	fs afero.Fs,
	width, height uint,
	httpContext context.Context,
) ([]byte, error) {
	// Determine runTimeout based on context's deadline
	runTimeout := 10 * time.Second // default run timeout

	// Check if the context has a deadline and adjust the runTimeout accordingly
	if deadline, ok := httpContext.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < runTimeout {
			runTimeout = remaining
		}
	}

	var fileBytes []byte

	// Execute thumbnail generation with lock and timeout using RunWithLockTimeout
	lockAcquired, err := ctx.locks.VideoThumbnailGenerationLock.RunWithLockTimeout(
		resource.ID,    // Assuming resourceId is of type T comparable (uint)
		30*time.Second, // lockTimeout
		runTimeout,     // runTimeout
		func() error {
			// Perform thumbnail generation
			file, err := fs.Open(resource.GetCleanLocation())
			if err != nil {
				return fmt.Errorf("failed to open video file: %w", err)
			}
			defer file.Close()

			select {
			case <-httpContext.Done():
				return httpContext.Err()
			default:
			}

			// Create thumbnail from video
			resultBuffer := bytes.NewBuffer([]byte{})
			if err := ctx.createThumbFromVideo(httpContext, file, resultBuffer); err != nil {
				return fmt.Errorf("failed to create thumbnail from video: %w", err)
			}

			select {
			case <-httpContext.Done():
				return httpContext.Err()
			default:
			}

			// Decode the generated thumbnail image
			originalImage, _, err := image.Decode(resultBuffer)
			if err != nil {
				return fmt.Errorf("failed to decode thumbnail image: %w", err)
			}

			// Resize the image to desired dimensions
			newImage := resize.Resize(width, height, originalImage, resize.Lanczos3)

			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, newImage, nil); err != nil {
				return fmt.Errorf("failed to encode resized image: %w", err)
			}

			// Read the resized image bytes
			fileBytes, err = io.ReadAll(&buf)
			if err != nil {
				return fmt.Errorf("failed to read resized image bytes: %w", err)
			}

			// Indicate success
			return nil
		},
	)

	// Check if the lock was successfully acquired
	if !lockAcquired {
		return nil, errors.New("failed to acquire video thumbnail generation lock")
	}

	// Handle potential errors from thumbnail generation
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("video thumbnail generation timed out")
		}
		return nil, fmt.Errorf("video thumbnail generation error: %w", err)
	}

	return fileBytes, nil
}

// createThumbFromVideo generates a thumbnail from a video file.
// It attempts to create a thumbnail at 1 second, and if that fails or returns no data,
// it retries at 0 seconds.
//
// Parameters:
// - ctx: The context to manage cancellation and timeouts.
// - file: The video file to generate a thumbnail from. It must implement io.ReadSeeker.
// - resultBuffer: The buffer to write the thumbnail image data to.
//
// Returns:
// - error: An error if thumbnail generation fails or is canceled.
func (ctx *MahresourcesContext) createThumbFromVideo(
	context context.Context,
	file io.ReadSeeker,
	resultBuffer *bytes.Buffer,
) error {
	// First attempt to create thumbnail at 1 second
	err := ctx.createThumbFromVideoAtGivenTime(context, file, resultBuffer, 1)

	// If the first attempt fails or returns no data, try again at 0 seconds
	if err != nil || resultBuffer.Len() == 0 {
		resultBuffer.Reset()

		// Reset the reader back to the beginning
		_, err := file.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek video file: %w", err)
		}

		err = ctx.createThumbFromVideoAtGivenTime(context, file, resultBuffer, 0)
		if err != nil {
			return fmt.Errorf("failed to create thumbnail at 0 seconds: %w", err)
		}
	}

	return nil
}

// createThumbFromVideoAtGivenTime generates a thumbnail from a specific time in the video.
//
// Parameters:
// - ctx: The context to manage cancellation and timeouts.
// - file: The video file to generate a thumbnail from. It must implement io.Reader.
// - resultBuffer: The buffer to write the thumbnail image data to.
// - secondsIn: The time (in seconds) in the video to capture the frame.
//
// Returns:
// - error: An error if thumbnail generation fails or is canceled.
func (ctx *MahresourcesContext) createThumbFromVideoAtGivenTime(
	context context.Context,
	file io.Reader,
	resultBuffer *bytes.Buffer,
	secondsIn int,
) error {
	// Construct the ffmpeg command with context support
	cmd := exec.CommandContext(context, ctx.Config.FfmpegPath,
		"-i", "pipe:0", // input from stdin
		"-ss", fmt.Sprintf("00:00:%02d", secondsIn), // capture frame at secondsIn
		"-vframes", "1", // grab one frame
		"-vf", "scale=640:-1", // scale the image if needed
		"-c:v", "png", // encode to PNG
		"-f", "image2pipe", // output format
		"pipe:1", // output to stdout (pipe)
	)

	// Pipe the video data to ffmpeg's stdin
	cmd.Stdin = file

	// Set ffmpeg's stdout to write to resultBuffer
	cmd.Stdout = resultBuffer

	// Optionally, capture stderr for debugging purposes
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the ffmpeg command
	err := cmd.Run()
	if err != nil {
		// Check if the error is due to context cancellation
		if context.Err() != nil {
			return context.Err()
		}
		return fmt.Errorf("ffmpeg error: %w, stderr: %s", err, stderr.String())
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

func (ctx *MahresourcesContext) RecalculateResourceDimensions(query *query_models.EntityIdQuery) error {
	var resource models.Resource

	if err := ctx.db.First(&resource, query.ID).Error; err != nil {
		return err
	}

	fs, storageErr := ctx.GetFsForStorageLocation(resource.StorageLocation)

	if storageErr != nil {
		return storageErr
	}

	file, openErr := fs.Open(resource.GetCleanLocation())

	if openErr != nil {
		return openErr
	}

	defer file.Close()

	img, _, err := image.Decode(file)

	if err != nil {
		return err
	}

	bounds := img.Bounds()

	resource.Width = uint(bounds.Max.X)
	resource.Height = uint(bounds.Max.Y)

	return ctx.db.Save(&resource).Error
}

func (ctx *MahresourcesContext) SetResourceDimensions(resourceId uint, width, height uint) error {
	var resource models.Resource

	if err := ctx.db.First(&resource, resourceId).Error; err != nil {
		return err
	}

	resource.Width = width
	resource.Height = height

	return ctx.db.Save(&resource).Error
}

func (ctx *MahresourcesContext) BulkAddMetaToResources(query *query_models.BulkEditMetaQuery) error {
	var resource models.Resource

	var expr clause.Expr

	if ctx.Config.DbType == constants.DbTypePosgres {
		expr = gorm.Expr("meta || ?", query.Meta)
	} else {
		expr = gorm.Expr("json_patch(meta, ?)", query.Meta)
	}

	return ctx.db.
		Model(&resource).
		Where("id in ?", query.ID).
		Update("Meta", expr).Error
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
		} else if transactionCtx.Config.DbType == constants.DbTypeSqlite {
			if err := tx.Exec("update resources set meta = json_patch(meta, ?) where id = ?", backups, winner.ID).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (ctx *MahresourcesContext) RotateResource(resourceId uint, degrees int) error {
	var resource models.Resource

	if err := ctx.db.First(&resource, resourceId).Error; err != nil {
		return err
	}

	if !resource.IsImage() {
		return errors.New("not an image")
	}

	f, err := ctx.fs.Open(resource.GetCleanLocation())

	if err != nil {
		return err
	}

	img, _, err := image.Decode(f)

	if err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	rotatedImage := transform.Rotate(img, float64(degrees), &transform.RotationOptions{ResizeBounds: true})

	var buf bytes.Buffer
	if err := imgio.JPEGEncoder(100)(&buf, rotatedImage); err != nil {
		return err
	}

	newFile, err := ctx.fs.Create(resource.GetCleanLocation() + ".rotated")
	if err != nil {
		return err
	}

	if _, err := io.Copy(newFile, &buf); err != nil {
		return err
	}

	if err := ctx.fs.Remove(resource.GetCleanLocation()); err != nil {
		return err
	}

	if err := newFile.Close(); err != nil {
		return err
	}

	if err := ctx.fs.Rename(resource.GetCleanLocation()+".rotated", resource.GetCleanLocation()); err != nil {
		return err
	}

	// delete the thumbnail(s)
	if err := ctx.db.Where("resource_id = ?", resourceId).Delete(&models.Preview{}).Error; err != nil {
		return err
	}

	return ctx.RecalculateResourceDimensions(&query_models.EntityIdQuery{ID: resourceId})
}
