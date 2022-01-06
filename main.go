package main

import (
	"github.com/joho/godotenv"
	"log"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"
)

func main() {
	// you may have no .env, it's okay
	_ = godotenv.Load(".env")

	context, db, mainFs := application_context.CreateContext()

	if err := db.AutoMigrate(
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
	); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	util.AddInitialData(db)

	indexQueries := [...]string{
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__note_id ON resource_notes(note_id)",
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__resource_id ON resource_notes(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id ON groups_related_resources(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__group_id ON groups_related_resources(group_id)",
	}

	for _, query := range indexQueries {
		if err := db.Exec(query).Error; err != nil {
			log.Fatalf("Error when creating index: %v", err)
		}
	}

	log.Fatal(server.CreateServer(context, mainFs, context.Config.AltFileSystems).ListenAndServe())
}
