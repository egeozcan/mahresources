package context

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/gabriel-vasile/mimetype"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"io/ioutil"
	"mahresources/database_scopes"
	"mahresources/http_query"
	"mahresources/models"
	"path"
)

func (ctx *MahresourcesContext) GetResource(id int64) (*models.Resource, error) {
	var resource models.Resource
	ctx.db.Preload(clause.Associations).First(&resource, id)

	if resource.ID == 0 {
		return nil, errors.New("could not find the resource")
	}

	return &resource, nil
}

func (ctx *MahresourcesContext) GetResourceCount(query *http_query.ResourceQuery) (int64, error) {
	var resource models.Resource
	var count int64
	ctx.db.Scopes(database_scopes.ResourceQuery(query)).Model(&resource).Count(&count)

	return count, nil
}

func (ctx *MahresourcesContext) GetResources(offset, maxResults int, query *http_query.ResourceQuery) (*[]models.Resource, error) {
	var resources []models.Resource

	ctx.db.Scopes(database_scopes.ResourceQuery(query)).Limit(maxResults).Offset(int(offset)).Preload("Tags").Find(&resources)

	return &resources, nil
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

func (ctx *MahresourcesContext) AddResource(file File, fileName string, resourceQuery *http_query.ResourceCreator) (*models.Resource, error) {
	fileMime, err := mimetype.DetectReader(file)

	if err != nil {
		return nil, err
	}

	preview, err := base64.StdEncoding.DecodeString(resourceQuery.Preview)

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

	err = ctx.fs.MkdirAll(folder, 0777)

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

	name := fileName

	if resourceQuery.Name != "" {
		name = resourceQuery.Name
	}

	res := &models.Resource{
		Name:               name,
		Hash:               hash,
		HashType:           "SHA1",
		Location:           filePath,
		Meta:               resourceQuery.Meta,
		Category:           resourceQuery.Category,
		ContentType:        fileMime.String(),
		ContentCategory:    resourceQuery.ContentCategory,
		Preview:            preview,
		PreviewContentType: resourceQuery.PreviewContentType,
		FileSize:           int64(len(fileBytes)) << 3,
	}

	ctx.db.Save(res)

	return res, nil
}

func (ctx *MahresourcesContext) GetTagsForResources() (*[]models.Tag, error) {
	var tags []models.Tag
	ctx.db.Raw(`SELECT
					  Count(*)
					  , id
					  , name
					from tags t
					join resource_tags rt on t.id = rt.tag_id
					group by t.name, t.id
					order by count(*) desc
	`).Scan(&tags)

	return &tags, nil
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
