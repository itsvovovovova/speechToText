package main

import (
	"database/sql"
	"log"
	"math"
	"os"
	"speechToText/src/api"
	"speechToText/src/cache"
	"speechToText/src/config"
	"speechToText/src/db"
	"testing"
)

var (
	testStore    *db.Store
	testSQL      *sql.DB
	testHandlers *api.Handlers
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	sqlDB, err := db.NewDB(config.CurrentConfig.Database)
	if err != nil {
		if os.Getenv("STT_INTEGRATION") == "1" {
			log.Printf("integration database unavailable: %v", err)
			return 1
		}
		log.Printf("DB not available, DB-dependent tests will be skipped: %v", err)
	} else {
		testStore = db.NewStore(sqlDB)
		testSQL = sqlDB
		defer sqlDB.Close()
	}

	sessionProvider := cache.NewRedisSessionProvider(config.CurrentConfig.Redis.Host)
	defer sessionProvider.Close()
	sessionManager := cache.NewRedisSessionManager("session_id", sessionProvider, int64(math.Pow10(5)))

	testHandlers = api.NewHandlers(testStore, sessionManager)

	return m.Run()
}
