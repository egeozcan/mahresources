package main

import (
	"os"

	"github.com/joho/godotenv"
	"log"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"
)

func main() {
	// you may have no .env, it's okay
	_ = godotenv.Load(".env")

	context, db, mainFs := application_context.CreateContext()

	if err := db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
	); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	util.AddInitialData(db)

	indexQueries := [...]string{
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__note_id ON resource_notes(note_id)",
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__resource_id ON resource_notes(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id ON groups_related_resources(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id___hash ON groups_related_resources USING HASH (resource_id);",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__group_id ON groups_related_resources(group_id)",
	}

	indexQueriesSqlite := [...]string{
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__note_id ON resource_notes(note_id)",
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__resource_id ON resource_notes(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id ON groups_related_resources(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id___hash ON groups_related_resources(resource_id);",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__group_id ON groups_related_resources(group_id)",
	}

	if context.Config.DbType == constants.DbTypePosgres {
		for _, query := range indexQueries {
			if err := db.Exec(query).Error; err != nil {
				log.Fatalf("Error when creating index: %v", err)
			}
		}
	} else {
		for _, query := range indexQueriesSqlite {
			if err := db.Exec(query).Error; err != nil {
				log.Fatalf("Error when creating index: %v", err)
			}
		}
	}

	// Initialize Full-Text Search (skip with SKIP_FTS=1 env var)
	if os.Getenv("SKIP_FTS") != "1" {
		if err := context.InitFTS(); err != nil {
			log.Printf("Warning: FTS setup failed, falling back to LIKE-based search: %v", err)
		}
	} else {
		log.Println("FTS setup skipped (SKIP_FTS=1)")
	}

	log.Fatal(server.CreateServer(context, mainFs, context.Config.AltFileSystems).ListenAndServe())
}
