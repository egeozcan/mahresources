package main

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
	"os"
	"path"
)

type File interface {
	io.Reader
	io.Seeker
	io.Closer
}

type mahresourcesContext struct {
	Filesystem afero.Fs
	db *gorm.DB
}

func newMahresourcesContext(filesystem afero.Fs, db *gorm.DB) *mahresourcesContext {
	return &mahresourcesContext{Filesystem: filesystem, db: db}
}

func (ctx *mahresourcesContext) createAlbum(name string) (*Album, error) {
	if name == "" {
		return nil, errors.New("album name needed")
	}
	album := Album{
		Name: name,
	}
	ctx.db.Create(&album)
	return &album, nil
}

func (ctx *mahresourcesContext) getAlbum(id uint) (*Album, error) {
	var album Album
	ctx.db.Preload(clause.Associations).First(&album, id)

	if album.ID == 0 {
		return nil, errors.New("could not load album")
	}

	return &album, nil
}

func (ctx *mahresourcesContext) getAlbums(offset, maxResults int) (*[]Album, error) {
	var albums []Album
	ctx.db.Limit(maxResults).Offset(int(offset)).Preload("Tags").Find(&albums)

	if len(albums) == 0 {
		return nil, errors.New("no albums found")
	}

	return &albums, nil
}

func (ctx *mahresourcesContext) getResource(id int64) (*Resource, error) {
	var resource Resource
	ctx.db.Preload(clause.Associations).First(&resource, id)

	if resource.ID == 0 {
		return nil, errors.New("could not find the resource")
	}

	return &resource, nil
}

func (ctx *mahresourcesContext) addResourceToAlbum(resId, albumId int64) (*Resource, error) {
	var resource Resource
	ctx.db.First(&resource, resId)
	var album Album
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

func (ctx *mahresourcesContext) addResource(file File, fileName string) (*Resource, error) {
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

	err = ctx.Filesystem.MkdirAll(folder, os.ModeDir)

	if err != nil {
		return nil, err
	}

	filePath := path.Join(folder, hash+fileMime.Extension())
	stat, statError := ctx.Filesystem.Stat(filePath)

	if statError == nil && stat != nil {
		return nil, errors.New("file already exists")
	}

	savedFile, err := ctx.Filesystem.Create(filePath)
	if err != nil {
		return nil, err
	}
	defer savedFile.Close()

	_, err = savedFile.Write(fileBytes)
	if err != nil {
		return nil, err
	}

	res := &Resource{
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

func (ctx *mahresourcesContext) addThumbnailToResource(file File, resourceId int64) (*Resource, error) {
	var resource Resource

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