package models

import (
	"errors"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"io/fs"
	"log"
	"os"
)

func CreateDatabaseConnection(dbType, dsn, logType string) (*gorm.DB, error) {
	var dbLogger logger.Interface
	var db *gorm.DB

	config := logger.Config{
		SlowThreshold: 0,
		LogLevel:      logger.Info,
		Colorful:      true,
	}

	switch logType {
	case "STDOUT":
		logWriter := log.New(os.Stdout, "\r\n", log.LstdFlags)
		logWriter.Println("Logging to STDOUT")
		dbLogger = logger.New(
			logWriter,
			config,
		)
	case "":
		dbLogger = logger.New(
			log.New(io.Discard, "", 0),
			logger.Config{},
		)
	default:
		open, err := os.OpenFile(logType, os.O_WRONLY, fs.ModeAppend)

		if err != nil {
			return nil, err
		}

		dbLogger = logger.New(
			log.New(open, "\r\n", log.LstdFlags), // io writer
			config,
		)
	}

	switch dbType {
	case "POSTGRES":
		if pgDb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: dbLogger,
		}); err != nil {
			return nil, err
		} else {
			db = pgDb
		}
	case "SQLITE":
		if sqliteDb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
			Logger: dbLogger,
		}); err != nil {
			return nil, err
		} else {
			db = sqliteDb
		}

		db.Exec("PRAGMA foreign_keys = ON;")
	default:
		return nil, errors.New("please set the DB_TYPE env var to SQLITE or POSTGRES")
	}

	if err := db.AutoMigrate(
		&Resource{},
		&Note{},
		&Tag{},
		&Group{},
		&Category{},
		&NoteType{},
		&Preview{},
		&GroupRelation{},
		&GroupRelationType{},
	); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	return db, nil
}
