package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type Client struct{}

var tableSchemas = [...]string{
	`CREATE TABLE folders (
		"id"     INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"name"   TEXT,
		"parent" INTEGER,
		FOREIGN KEY("parent") REFERENCES folders("id")
	);`,
	`CREATE TABLE files (
		"id"     INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"name"   TEXT,
		"url"    TEXT,
		"size"   INTEGER,
		"mode"   INTEGER,
		"parent" INTEGER,
		FOREIGN KEY("parent") REFERENCES folders("id")
	);`,
}

// InitDB ...
// Initializes tables
// Supposed to be called on the very first run
func InitDB(path string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return fmt.Errorf("db: unable to open db at %v: %v", path, err)
	}
	defer db.Close()


	for _, sqlStr := range tableSchemas {
		st, err := db.Prepare(sqlStr)
		if err != nil {
			return fmt.Errorf("db: error in initialization query: %v", err)
		}

		_, err = st.Exec()
		if err != nil {
			return fmt.Errorf("db: unable to execute initialization query: %v", err)
		}
	}

	return nil
}

func NewClient(path string) (*Client, error) {
	return &Client{}, nil
}

func (c *Client) Close() {

}
