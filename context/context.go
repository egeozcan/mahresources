package context

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"github.com/gabriel-vasile/mimetype"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"io/ioutil"
	"mahresources/models"
	"os"
	"path"
)

type File interface {
	io.Reader
	io.Seeker
	io.Closer
}

type MahresourcesContext struct {
	fs afero.Fs
	db *gorm.DB
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB) *MahresourcesContext {
	return &MahresourcesContext{fs: filesystem, db: db}
}

func (ctx *MahresourcesContext) GetResource(id int64) (*models.Resource, error) {
	var resource models.Resource
	ctx.db.Preload(clause.Associations).First(&resource, id)

	if resource.ID == 0 {
		return nil, errors.New("could not find the resource")
	}

	return &resource, nil
}

func (ctx *MahresourcesContext) AddResourceToAlbum(resId, albumId int64) (*models.Resource, error) {
	var resource models.Resource
	ctx.db.First(&resource, resId)
	var album models.Album
	ctx.db.First(&album, albumId)

	err := ctx.db.Model(&album).Association("Resources").Append(resource)

	if err != nil {
		return nil, err
	}

	if resource.ID == 0 || album.ID == 0 {
		return nil, errors.New("could not find relevant resources")
	}

	return &resource, nil
}

func (ctx *MahresourcesContext) AddResource(file File, fileName string) (*models.Resource, error) {
	fileMime, err := mimetype.DetectReader(file)

	if err != nil {
		return nil, err
	}

	_, err = file.Seek(0, io.SeekStart)

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
	folder := "/resources/" + hash[0:2] + "/" + hash[2:4] + "/" + hash[4:6] + "/"

	err = ctx.fs.MkdirAll(folder, os.ModeDir)

	if err != nil {
		return nil, err
	}

	filePath := path.Join(folder, hash+fileMime.Extension())
	stat, statError := ctx.fs.Stat(filePath)

	if statError == nil && stat != nil {
		return nil, errors.New("file already exists")
	}

	savedFile, err := ctx.fs.Create(filePath)
	if err != nil {
		return nil, err
	}
	defer savedFile.Close()

	_, err = savedFile.Write(fileBytes)
	if err != nil {
		return nil, err
	}

	res := &models.Resource{
		Name:               fileName,
		Hash:               hash,
		HashType:           "SHA1",
		Location:           filePath,
		Meta:               "",
		Category:           "",
		ContentType:        fileMime.String(),
		ContentCategory:    "",
		Preview:            nil,
		PreviewContentType: "",
		FileSize:           int64(len(fileBytes)) << 3,
	}

	return res, nil
}

func (ctx *MahresourcesContext) AddThumbnailToResource(file File, resourceId int64) (*models.Resource, error) {
	var resource models.Resource

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		tx.First(&resource, resourceId)

		if resource.ID == 0 {
			return errors.New("not found")
		}

		fileMime, err := mimetype.DetectReader(file)
		if err != nil {
			return err
		}

		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}

		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			return err
		}

		resource.Preview = fileBytes
		resource.PreviewContentType = fileMime.String()

		tx.Save(resource)

		return nil
	})

	return &resource, err
}

func (ctx *MahresourcesContext) AddThumbnailToAlbum(file File, albumId int64) (*models.Album, error) {
	var album models.Album

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		tx.First(&album, albumId)

		if album.ID == 0 {
			return errors.New("not found")
		}

		fileMime, err := mimetype.DetectReader(file)
		if err != nil {
			return err
		}

		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}

		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			return err
		}

		album.Preview = fileBytes
		album.PreviewContentType = fileMime.String()

		tx.Save(album)

		return nil
	})

	return &album, err
}
