package db

import (
	"database/sql"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

const defaultDSN = "root:root@tcp(127.0.0.1:3306)/twopc?parseTime=true"

func Open() (*sql.DB, error) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = defaultDSN
	}
	database, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}
