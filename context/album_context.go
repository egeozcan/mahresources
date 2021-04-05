package context

import (
	"encoding/base64"
	"errors"
	"gorm.io/gorm/clause"
	"mahresources/context/database_scopes"
	"mahresources/http_utils/http_query"
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
		Meta:               albumQuery.Meta,
		Preview:            preview,
		PreviewContentType: albumQuery.PreviewContentType,
		OwnerId:            albumQuery.OwnerId,
	}
	ctx.db.Create(&album)
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
