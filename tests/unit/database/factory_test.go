package database_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/stretchr/testify/assert"
)

func TestNewDatabase_Postgres(t *testing.T) {
	db := database.NewDatabase("postgres", "host=localhost port=5432 user=test dbname=test sslmode=disable")
	assert.IsType(t, &database.PostgresDB2{}, db)
}

func TestNewDatabase_SQLite(t *testing.T) {
	db := database.NewDatabase("sqlite", ":memory:")
	assert.IsType(t, &database.SQLiteDB{}, db)
}

func TestNewDatabase_UnknownDriverDefaultsToSQLite(t *testing.T) {
	db := database.NewDatabase("unknown", ":memory:")
	assert.IsType(t, &database.SQLiteDB{}, db)
}

func TestPostgresDB2_Connect_InvalidDSN(t *testing.T) {
	db := database.NewPostgresDB("invalid dsn")
	ctx := context.Background()
	err := db.Connect(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "postgres ping")
}

func TestPostgresDB2_Close_NoConnection(t *testing.T) {
	db := database.NewPostgresDB("")
	err := db.Close()
	assert.NoError(t, err)
}

func TestPostgresDB2_DB_Nil(t *testing.T) {
	db := database.NewPostgresDB("")
	assert.Nil(t, db.DB())
}
