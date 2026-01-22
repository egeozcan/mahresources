package application_context

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/interfaces"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/spf13/afero"
	"gorm.io/gorm"
)

// timeoutReader wraps an io.Reader and returns an error if no data is read within the timeout period
type timeoutReader struct {
	reader      io.Reader
	idleTimeout time.Duration
	done        chan struct{}
	mu          sync.Mutex
	lastRead    time.Time
	err         error
}

func newTimeoutReader(r io.Reader, idleTimeout time.Duration) *timeoutReader {
	tr := &timeoutReader{
		reader:      r,
		idleTimeout: idleTimeout,
		lastRead:    time.Now(),
		done:        make(chan struct{}),
	}
	go tr.watchTimeout()
	return tr
}

func (tr *timeoutReader) watchTimeout() {
	// Check frequently enough to detect timeouts promptly, but not so frequently as to waste CPU
	checkInterval := tr.idleTimeout / 10
	if checkInterval < 100*time.Millisecond {
		checkInterval = 100 * time.Millisecond
	}
	if checkInterval > time.Second {
		checkInterval = time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-tr.done:
			return
		case <-ticker.C:
			tr.mu.Lock()
			elapsed := time.Since(tr.lastRead)
			if elapsed > tr.idleTimeout {
				tr.err = fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
				tr.mu.Unlock()
				return
			}
			tr.mu.Unlock()
		}
	}
}

type readResult struct {
	n   int
	err error
}

func (tr *timeoutReader) Read(p []byte) (n int, err error) {
	// Check for existing error
	tr.mu.Lock()
	if tr.err != nil {
		err := tr.err
		tr.mu.Unlock()
		return 0, err
	}
	tr.mu.Unlock()

	// Run read in goroutine so we can interrupt it on timeout.
	// Note: On timeout, this goroutine may outlive the Read call. It will exit
	// when the underlying reader returns (e.g., when the HTTP connection closes).
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := tr.reader.Read(p)
		resultCh <- readResult{n, err}
	}()

	// Wait for read to complete or timeout
	for {
		select {
		case result := <-resultCh:
			if result.n > 0 {
				tr.mu.Lock()
				tr.lastRead = time.Now()
				tr.mu.Unlock()
			}
			return result.n, result.err
		case <-tr.done:
			return 0, fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
		default:
			tr.mu.Lock()
			err := tr.err
			tr.mu.Unlock()
			if err != nil {
				return 0, err
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (tr *timeoutReader) Close() error {
	close(tr.done)
	return nil
}

// createRemoteResourceHTTPClient creates an HTTP client with the given timeouts
func createRemoteResourceHTTPClient(connectTimeout, overallTimeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: overallTimeout,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: connectTimeout}).DialContext,
			TLSHandshakeTimeout:   connectTimeout / 2, // TLS handshake gets half the connect timeout
			ResponseHeaderTimeout: connectTimeout,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

func (ctx *MahresourcesContext) AddRemoteResource(resourceQuery *query_models.ResourceFromRemoteCreator) (*models.Resource, error) {
	urls := strings.Split(resourceQuery.URL, "\n")
	var firstResource *models.Resource
	var firstError error

	// Get timeout values from config
	connectTimeout := ctx.Config.RemoteResourceConnectTimeout
	idleTimeout := ctx.Config.RemoteResourceIdleTimeout
	overallTimeout := ctx.Config.RemoteResourceOverallTimeout

	httpClient := createRemoteResourceHTTPClient(connectTimeout, overallTimeout)

	setError := func(err error) {
		if firstError == nil {
			firstError = err
		}
		print(err)
	}

	for _, url := range urls {
		(func(url string) {
			resp, err := httpClient.Get(url)

			if err != nil {
				setError(err)
				return
			}

			defer resp.Body.Close()

			// Wrap response body with timeout reader to detect stalled transfers
			timeoutBody := newTimeoutReader(resp.Body, idleTimeout)
			defer timeoutBody.Close()

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

			res, err := ctx.AddResource(timeoutBody, resourceQuery.FileName, &query_models.ResourceCreator{
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

	ctx.InvalidateSearchCacheByType(EntityTypeResource)
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

	// Acquire per-hash lock to prevent race condition where two simultaneous uploads
	// with the same hash both pass the "existing resource" check before either commits
	ctx.locks.ResourceHashLock.Acquire(hash)
	defer ctx.locks.ResourceHashLock.Release(hash)

	var existingResource models.Resource

	if existingNotFoundErr := tx.Where("hash = ?", hash).Preload("Groups").First(&existingResource).Error; existingNotFoundErr == nil {
		if existingResource.OwnerId != nil && resourceQuery.OwnerId == *existingResource.OwnerId {
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
		groups := BuildAssociationSlice(resourceQuery.Groups, GroupFromID)

		if createGroupsErr := tx.Model(&res).Association("Groups").Append(&groups); createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	if len(resourceQuery.Notes) > 0 {
		notes := BuildAssociationSlice(resourceQuery.Notes, NoteFromID)

		if createNotesErr := tx.Model(&res).Association("Notes").Append(&notes); createNotesErr != nil {
			tx.Rollback()
			return nil, createNotesErr
		}
	}

	if len(resourceQuery.Tags) > 0 {
		tags := BuildAssociationSlice(resourceQuery.Tags, TagFromID)

		if createTagsErr := tx.Model(&res).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return nil, createTagsErr
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	ctx.InvalidateSearchCacheByType(EntityTypeResource)
	return res, nil
}
