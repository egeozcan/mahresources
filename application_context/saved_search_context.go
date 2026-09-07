package application_context

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"mahresources/listviews"
	"mahresources/models"

	"gorm.io/gorm"
)

var ErrSavedSearchInvalid = errors.New("invalid saved search")

func savedSearchName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return "", fmt.Errorf("%w: name must be 1–200 bytes", ErrSavedSearchInvalid)
	}
	return name, nil
}

func savedSearchURL(raw string) (string, *listviews.View, error) {
	if len(raw) > 64*1024 {
		return "", nil, fmt.Errorf("%w: URL is too long", ErrSavedSearchInvalid)
	}
	value, view, err := listviews.NormalizeSavedURL(raw)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s", ErrSavedSearchInvalid, err)
	}
	return value, view, nil
}

func decorateSavedSearch(search *models.SavedSearch) {
	if u, err := url.Parse(search.URL); err == nil {
		if view := listviews.Lookup(u.Path); view != nil {
			search.Layout = view.Layout
		}
	}
}

func (ctx *MahresourcesContext) GetSavedSearches(family string) ([]models.SavedSearch, error) {
	if family != "" && !listviews.ValidFamily(family) {
		return nil, fmt.Errorf("%w: unknown list family", ErrSavedSearchInvalid)
	}
	searches := []models.SavedSearch{}
	owner := ctx.actingUserID()
	if owner == 0 {
		return searches, nil
	}
	query := ctx.db.Where("user_id = ?", owner)
	if family != "" {
		query = query.Where("family = ?", family)
	}
	if err := query.Order("LOWER(name), id").Find(&searches).Error; err != nil {
		return nil, err
	}
	for i := range searches {
		decorateSavedSearch(&searches[i])
	}
	return searches, nil
}

func (ctx *MahresourcesContext) CreateSavedSearch(name, rawURL string) (*models.SavedSearch, error) {
	owner := ctx.actingUserID()
	if owner == 0 {
		return nil, ErrNoSettingsOwner
	}
	name, err := savedSearchName(name)
	if err != nil {
		return nil, err
	}
	value, view, err := savedSearchURL(rawURL)
	if err != nil {
		return nil, err
	}
	search := &models.SavedSearch{UserId: owner, Name: name, Family: view.Family, URL: value, Layout: view.Layout}
	return search, ctx.db.Create(search).Error
}

// Update only the supplied fields so a rename cannot overwrite a concurrent URL edit.
func (ctx *MahresourcesContext) UpdateSavedSearch(id uint, name, rawURL *string) (*models.SavedSearch, error) {
	owner := ctx.actingUserID()
	if owner == 0 {
		return nil, ErrNoSettingsOwner
	}
	var search models.SavedSearch
	if err := ctx.db.Where("id = ? AND user_id = ?", id, owner).First(&search).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if name != nil {
		value, err := savedSearchName(*name)
		if err != nil {
			return nil, err
		}
		updates["name"] = value
	}
	if rawURL != nil {
		value, view, err := savedSearchURL(*rawURL)
		if err != nil {
			return nil, err
		}
		if view.Family != search.Family {
			return nil, fmt.Errorf("%w: replacement must use the same list family", ErrSavedSearchInvalid)
		}
		updates["url"] = value
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("%w: supply name or url", ErrSavedSearchInvalid)
	}
	result := ctx.db.Model(&models.SavedSearch{}).Where("id = ? AND user_id = ?", id, owner).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if err := ctx.db.Where("id = ? AND user_id = ?", id, owner).First(&search).Error; err != nil {
		return nil, err
	}
	decorateSavedSearch(&search)
	return &search, nil
}

func (ctx *MahresourcesContext) DeleteSavedSearch(id uint) error {
	owner := ctx.actingUserID()
	if owner == 0 {
		return ErrNoSettingsOwner
	}
	result := ctx.db.Where("id = ? AND user_id = ?", id, owner).Delete(&models.SavedSearch{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
