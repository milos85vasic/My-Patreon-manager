package database

func NewDatabase(driver, dsn string) Database {
	switch driver {
	case "sqlite", "sqlite3":
		return NewSQLiteDB(dsn)
	case "postgres", "pg", "postgresql":
		return NewPostgresDB(dsn)
	default:
		return NewSQLiteDB(dsn)
	}
}
