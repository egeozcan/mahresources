package application_context

import (
	"errors"
	"fmt"
	"mahresources/models"
	"mahresources/plugin_system"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The cap this layer enforces is the one mah.kv publishes as max_value_size, so
// it is declared once, beside the KVStore interface this file implements.
const maxKVValueSize = plugin_system.MaxKVValueSize

// checkKVValueSize is shared by every write so an author meets one message
// whichever call they made. It names both numbers: the failure raises into
// their handler, so the message is all they get.
func checkKVValueSize(value string) error {
	if len(value) > maxKVValueSize {
		return fmt.Errorf("value size %d bytes exceeds maximum of %d bytes", len(value), maxKVValueSize)
	}
	return nil
}

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
	if err := checkKVValueSize(value); err != nil {
		return err
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

// PluginKVCompareAndSet writes value only while the stored value is still
// expected, and reports whether it wrote. A nil expected means "no row may exist
// yet", which is a different state from a row holding the JSON null a Lua nil
// stores, and an author needs both.
//
// The comparison happens in the database, in the statement that writes. Reading
// here and then calling PluginKVSet would be the lost update the operation
// exists to close, because PluginKVSet is an unconditional upsert and nothing
// holds the key between the read and the write.
//
// One plugin's Lua is single-threaded within a process -- every entry into it
// takes that plugin's VM lock and holds it for the whole call -- so a read and
// a write inside one call are already covered. What that hold does not cover is
// where the lost update actually comes from, and neither arrangement is exotic.
// The read and the write land in separate calls: the function given to
// mah.start_job and a drained mah.http callback both run after the call that
// registered them released the lock, so no lock ever spans the pair. Or the
// deployment runs more than one process, each with its own VM and its own lock
// and no ordering between them at all.
//
// Two statements, because the two expectations are two different conditions and
// neither is the other in disguise. An insert that does nothing on conflict is
// exactly "the row must not exist"; a conditional update is exactly "the row
// exists and holds this". One upsert cannot say the second: with no row to
// conflict with it inserts, so a value expectation against a key that was
// deleted underneath the caller would report a replacement of something that
// was never there. Both statements are single and both answer through
// RowsAffected, which SQLite and Postgres agree on.
func (ctx *MahresourcesContext) PluginKVCompareAndSet(pluginName, key string, expected *string, value string) (bool, error) {
	if err := checkKVValueSize(value); err != nil {
		return false, err
	}

	if expected == nil {
		kv := models.PluginKV{
			PluginName: pluginName,
			Key:        key,
			Value:      value,
		}
		// The same conflict target PluginKVSet uses, which is the real unique
		// index: naming it rather than taking any conflict keeps this about the
		// key, not about swallowing whatever else the row could violate.
		res := ctx.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "plugin_name"}, {Name: "key"}},
			DoNothing: true,
		}).Create(&kv)
		if res.Error != nil {
			return false, res.Error
		}
		return res.RowsAffected == 1, nil
	}

	res := ctx.db.Model(&models.PluginKV{}).
		Where("plugin_name = ? AND key = ? AND value = ?", pluginName, key, *expected).
		Update("value", value)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (ctx *MahresourcesContext) PluginKVDelete(pluginName, key string) error {
	return ctx.db.Where("plugin_name = ? AND key = ?", pluginName, key).
		Delete(&models.PluginKV{}).Error
}

func (ctx *MahresourcesContext) PluginKVList(pluginName, prefix string) ([]string, error) {
	var keys []string
	q := ctx.db.Model(&models.PluginKV{}).Where("plugin_name = ?", pluginName)
	if prefix != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(prefix)
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
