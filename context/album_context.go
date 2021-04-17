package context

import (
	"encoding/base64"
	"errors"
	"github.com/gabriel-vasile/mimetype"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"io/ioutil"
	"mahresources/database_scopes"
	"mahresources/http_query"
	"mahresources/models"
)

func (ctx *MahresourcesContext) CreateAlbum(albumQuery *http_query.AlbumCreator) (*models.Album, error) {
	if albumQuery.Name == "" {
		return nil, errors.New("album name needed")
	}

	preview, err := base64.StdEncoding.DecodeString(albumQuery.Preview)

	if err != nil {
		return nil, err
	}

	album := models.Album{
		Name:               albumQuery.Name,
		Description:        albumQuery.Description,
		Meta:               albumQuery.Meta,
		Preview:            preview,
		PreviewContentType: albumQuery.PreviewContentType,
		OwnerId:            albumQuery.OwnerId,
	}
	ctx.db.Create(&album)

	if len(albumQuery.People) > 0 {
		people := make([]models.Person, len(albumQuery.People))
		for i, v := range albumQuery.People {
			people[i] = models.Person{
				Model: gorm.Model{ID: v},
			}
		}
		createPeopleErr := ctx.db.Model(&album).Association("People").Append(&people)

		if createPeopleErr != nil {
			return nil, createPeopleErr
		}
	}

	if len(albumQuery.Tags) > 0 {
		tags := make([]models.Tag, len(albumQuery.Tags))
		for i, v := range albumQuery.Tags {
			tags[i] = models.Tag{
				Model: gorm.Model{ID: v},
			}
		}
		createTagsErr := ctx.db.Model(&album).Association("Tags").Append(&tags)

		if createTagsErr != nil {
			return nil, createTagsErr
		}
	}

	return &album, nil
}

func (ctx *MahresourcesContext) GetAlbum(id uint) (*models.Album, error) {
	var album models.Album
	ctx.db.Preload(clause.Associations).First(&album, id)

	if album.ID == 0 {
		return nil, errors.New("could not load album")
	}

	return &album, nil
}

func (ctx *MahresourcesContext) GetAlbums(offset, maxResults int, query *http_query.AlbumQuery) (*[]models.Album, error) {
	var albums []models.Album

	ctx.db.Scopes(database_scopes.AlbumQuery(query)).Limit(maxResults).Offset(int(offset)).Preload("Tags").Find(&albums)

	return &albums, nil
}

func (ctx *MahresourcesContext) GetAlbumsWithIds(ids []uint) (*[]models.Album, error) {
	var albums []models.Album

	ctx.db.Find(&albums, ids)

	return &albums, nil
}

func (ctx *MahresourcesContext) GetAlbumCount(query *http_query.AlbumQuery) (int64, error) {
	var album models.Album
	var count int64
	ctx.db.Scopes(database_scopes.AlbumQuery(query)).Model(&album).Count(&count)

	return count, nil
}

func (ctx *MahresourcesContext) GetTagsForAlbums() (*[]models.Tag, error) {
	var tags []models.Tag
	ctx.db.Raw(`SELECT
					  Count(*)
					  , id
					  , name
					from tags t
					join album_tags at on t.id = at.tag_id
					group by t.name, t.id
					order by count(*) desc
	`).Scan(&tags)

	return &tags, nil
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
