package application_context

import (
	"errors"
	"fmt"
	"mahresources/models"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxKVValueSize = 8 * 1024 * 1024 // 8MB

func (ctx *MahresourcesContext) PluginKVGet(pluginName, key string) (string, bool, error) {
	var kv models.PluginKV
	err := ctx.db.Where("plugin_name = ? AND key = ?", pluginName, key).First(&kv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return kv.Value, true, nil
}

func (ctx *MahresourcesContext) PluginKVSet(pluginName, key, value string) error {
	if len(value) > maxKVValueSize {
		return fmt.Errorf("value size %d bytes exceeds maximum of %d bytes", len(value), maxKVValueSize)
	}
	kv := models.PluginKV{
		PluginName: pluginName,
		Key:        key,
		Value:      value,
	}
	return ctx.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_name"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&kv).Error
}

func (ctx *MahresourcesContext) PluginKVDelete(pluginName, key string) error {
	return ctx.db.Where("plugin_name = ? AND key = ?", pluginName, key).
		Delete(&models.PluginKV{}).Error
}

func (ctx *MahresourcesContext) PluginKVList(pluginName, prefix string) ([]string, error) {
	var keys []string
	q := ctx.db.Model(&models.PluginKV{}).Where("plugin_name = ?", pluginName)
	if prefix != "" {
		escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(prefix)
		q = q.Where("key LIKE ? ESCAPE '\\'", escaped+"%")
	}
	if err := q.Order("key").Pluck("key", &keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

func (ctx *MahresourcesContext) PluginKVPurge(pluginName string) error {
	return ctx.db.Where("plugin_name = ?", pluginName).
		Delete(&models.PluginKV{}).Error
}
