package application_context

import (
	"mahresources/models"

	"gorm.io/gorm/clause"
)

func (ctx *MahresourcesContext) PluginKVGet(pluginName, key string) (string, bool, error) {
	var kv models.PluginKV
	err := ctx.db.Where("plugin_name = ? AND key = ?", pluginName, key).First(&kv).Error
	if err != nil {
		if err.Error() == "record not found" {
			return "", false, nil
		}
		return "", false, err
	}
	return kv.Value, true, nil
}

func (ctx *MahresourcesContext) PluginKVSet(pluginName, key, value string) error {
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
		q = q.Where("key LIKE ?", prefix+"%")
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
