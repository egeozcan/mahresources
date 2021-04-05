package database_scopes

import (
	"gorm.io/gorm"
	"log"
	"reflect"
	"regexp"
	"strings"
)

// @todo still experimenting here
func FilterByChildren(entity interface{}, property string, childrenIds []uint) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		t := reflect.TypeOf(entity)
		field, gotField := t.FieldByName(property)

		if !gotField {
			log.Fatal("Could not find the property " + property)
		}

		tag, gotTag := field.Tag.Lookup("gorm")

		if !gotTag {
			log.Fatal("Could not find the gorm tag in property " + property)
		}

		re := regexp.MustCompile(`many2many:[^;]+`)

		manyToManyTag := re.Find([]byte(tag))
		joinTable := strings.Split(string(manyToManyTag), ":")[1]
		childrenType := field.Type.Elem().Elem()
		childrenName := strings.ToLower(strings.Split(childrenType.String(), ".")[1])

		stmt := &gorm.Statement{DB: db}
		err := stmt.Parse(reflect.New(childrenType))

		if err != nil {
			log.Fatal("Could not parse statement")
		}

		parentStmt := &gorm.Statement{DB: db}
		err = stmt.Parse(entity)

		if err != nil {
			log.Fatal("Could not parse statement")
		}

		childrenTable := stmt.Schema.Table
		_ = parentStmt.Schema.Table //@todo

		return db.Where("EXISTS (SELECT 1 FROM "+childrenTable+" JOIN "+joinTable+" ON "+childrenName+"_id", childrenIds)
	}
}
