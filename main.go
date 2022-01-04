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

	log.Fatal(server.CreateServer(context, mainFs, context.Config.AltFileSystems).ListenAndServe())
}
