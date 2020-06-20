package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"github.com/paddlesteamer/hdn-drv/internal/common"
)

type Folder struct {
	ID     uint64
	Name   string
	Mode   uint8
	Parent uint64
}

type File struct {
	ID     uint64
	Name   string
	URL    string
	Size   uint64
	Mode   uint8
	Parent uint64
}

type Client struct {
	db *sql.DB
}

var tableSchemas = [...]string{
	`CREATE TABLE folders (
		"id"     INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"name"   TEXT NOT NULL,
		"mode"   INTEGER NOT NULL,
		"parent" INTEGER NOT NULL,
		UNIQUE("name", "parent"),
		FOREIGN KEY("parent") REFERENCES folders("id")
	);`,
	`CREATE TABLE files (
		"id"     INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"name"   TEXT NOT NULL,
		"url"    TEXT NOT NULL,
		"size"   INTEGER NOT NULL,
		"mode"   INTEGER NOT NULL,
		"parent" INTEGER NOT NULL,
		UNIQUE("name", "parent"),
		FOREIGN KEY("parent") REFERENCES folders("id")
	);`,
	`INSERT INTO folders VALUES (1, "", 493, 1);`, // root folder with mode 0755
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

// NewClient ...
// Returns a new database connection
func NewClient(path string) (*Client, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("db: unable to open db at %v: %v", path, err)
	}

	c := &Client{
		db: db,
	}
	return c, nil
}

// Close ...
// Closes database connection
func (c *Client) Close() {
	c.db.Close()
}

func (c *Client) SearchInFolders(parent uint64, name string) (*Folder, error) {
	query, err := c.db.Prepare("SELECT * FROM folders WHERE name=? and parent=?")
	if err != nil {
		return nil, fmt.Errorf("db: unable to prepare statement: %v", err)
	}

	folder := &Folder{}

	err = query.QueryRow(parent, name).Scan(&folder.ID, &folder.Name, &folder.Mode, &folder.Parent)
	switch {
	case err == sql.ErrNoRows:
		return nil, common.ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("db: there is an error in query: %v", err)
	}

	return folder, nil
}

func (c *Client) SearchInFiles(parent uint64, name string) (*File, error) {
	query, err := c.db.Prepare("SELECT * FROM files WHERE name=? and parent=?")
	if err != nil {
		return nil, fmt.Errorf("db: unable to prepare statement: %v", err)
	}

	file := &File{}

	err = query.QueryRow(parent, name).Scan(&file.ID, &file.Name, &file.URL,
		&file.Size, &file.Mode, &file.Parent)
	switch {
	case err == sql.ErrNoRows:
		return nil, common.ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("db: there is an error in query: %v", err)
	}

	return file, nil
}
