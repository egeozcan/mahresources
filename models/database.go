package models

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"io/fs"
	"log"
	"mahresources/constants"
	"os"
	"strings"
	"sync"
)

var registerOnce sync.Once

// registerSQLiteDriver registers a custom SQLite driver that applies PRAGMAs
// (foreign_keys, busy_timeout) on every new connection via ConnectHook.
// This ensures ALL connections in Go's connection pool have the correct settings,
// not just the first one.
func registerSQLiteDriver() {
	registerOnce.Do(func() {
		sql.Register("sqlite3_pragmas", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if _, err := conn.Exec("PRAGMA foreign_keys = ON", nil); err != nil {
					return err
				}
				if _, err := conn.Exec("PRAGMA busy_timeout = 10000", nil); err != nil {
					return err
				}
				return nil
			},
		})
	})
}

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
	case constants.DbTypePosgres:
		if pgDb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: dbLogger,
		}); err != nil {
			return nil, err
		} else {
			db = pgDb
		}
	case constants.DbTypeSqlite:
		registerSQLiteDriver()

		if sqliteDb, err := gorm.Open(&sqlite.Dialector{
			DriverName: "sqlite3_pragmas",
			DSN:        dsn,
		}, &gorm.Config{
			Logger: dbLogger,
		}); err != nil {
			return nil, err
		} else {
			db = sqliteDb
		}
	default:
		return nil, errors.New("please set the DB_TYPE env var to SQLITE or POSTGRES")
	}

	return db, nil
}

func CreateReadOnlyDatabaseConnection(dbType, dsn string) (*sqlx.DB, error) {
	if dbType == strings.ToLower(constants.DbTypeSqlite) {
		// Use the custom driver that sets busy_timeout on every connection
		registerSQLiteDriver()
		dbType = "sqlite3_pragmas"
	}

	return sqlx.Open(dbType, dsn)
}
