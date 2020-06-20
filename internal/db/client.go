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
		"name"   TEXT NOT NULL,
		"parent" INTEGER NOT NULL,
		FOREIGN KEY("parent") REFERENCES folders("id")
	);`,
	`CREATE TABLE files (
		"id"     INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"name"   TEXT NOT NULL,
		"url"    TEXT NOT NULL,
		"size"   INTEGER NOT NULL,
		"mode"   INTEGER NOT NULL,
		"parent" INTEGER NOT NULL,
		FOREIGN KEY("parent") REFERENCES folders("id")
	);`,
	`INSERT INTO folders VALUES (1, "", 1);`, // root folder
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
