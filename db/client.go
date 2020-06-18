package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type Client struct {}

// InitDB ...
// Initializes tables
// Supposed to be called on the very first run
func InitDB(path string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return fmt.Errorf("db: unable to open db at %v: %v", path, err)
	}
	defer db.Close()

	// TODO: include cloud storage, revision, etc.
	sqlStr := `CREATE TABLE files (
		"id" integer NOT NULL PRIMARY KEY AUTOINCREMENT,
		"name" TEXT,
		"path" TEXT,
		"type" TEXT
	);`

	st, err := db.Prepare(sqlStr)
	if err != nil {
		return fmt.Errorf("db: error in initialization query: %v", err)
	}

	_, err = st.Exec()
	if err != nil {
		return fmt.Errorf("db: unable to execute initialization query: %v")
	}

	return nil
}

func NewClient(path string) (*Client, error) {
	return &Client{}, nil
}

func (c *Client) Close() {

}