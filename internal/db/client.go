package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"github.com/paddlesteamer/hdn-drv/internal/common"
)

type Client struct {
	db *sql.DB
}

var tableSchemas = [...]string{
	`CREATE TABLE files (
		"inode"  INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"name"   TEXT NOT NULL,
		"url"    TEXT NOT NULL DEFAULT "",
		"size"   INTEGER NOT NULL DEFAULT 0,
		"mode"   INTEGER NOT NULL,
		"parent" INTEGER NOT NULL,
		"type"   INTEGER NOT NULL,
		UNIQUE("name", "parent"),
		FOREIGN KEY("parent") REFERENCES folders("id")
	);`,
	fmt.Sprintf(`INSERT INTO files(inode, name, mode, parent, type) VALUES (1, "", 493, 0, %d);`, common.DRV_FOLDER), // root folder with mode 0755
}

// InitDB initializes tables
// Supposed to be called on the very first run
func InitDB(path string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return fmt.Errorf("couldn't open db at %s: %v", path, err)
	}
	defer db.Close()

	for _, sqlStr := range tableSchemas {
		st, err := db.Prepare(sqlStr)
		if err != nil {
			return fmt.Errorf("error in query `%s`: %v", sqlStr, err)
		}

		_, err = st.Exec()
		if err != nil {
			return fmt.Errorf("couldn't execute initialization query: %v", err)
		}
	}

	return nil
}

// NewClient returns a new database connection
func NewClient(path string) (*Client, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("couldn't open db at %s: %v", path, err)
	}

	c := &Client{
		db: db,
	}
	return c, nil
}

// Close terminates database connection
func (c *Client) Close() {
	c.db.Close()
}

func (c *Client) Search(parent int64, name string) (*common.Metadata, error) {
	query, err := c.db.Prepare("SELECT * FROM files WHERE name=? and parent=?")
	if err != nil {
		return nil, fmt.Errorf("couldn't prepare statement: %v", err)
	}

	row, err := query.Query(parent, name)
	if err != nil {
		return nil, fmt.Errorf("there is an error in query: %v", err)
	}
	defer row.Close()

	if !row.Next() {
		return nil, common.ErrNotFound
	}

	return c.parseRow(row)
}

func (c *Client) Get(inode int64) (*common.Metadata, error) {
	query, err := c.db.Prepare("SELECT * FROM files WHERE inode=?")
	if err != nil {
		return nil, fmt.Errorf("couldn't prepare statement: %v", err)
	}

	row, err := query.Query(inode)
	if err != nil {
		return nil, fmt.Errorf("there is an error in query: %v", err)
	}
	defer row.Close()

	if !row.Next() {
		return nil, common.ErrNotFound
	}

	return c.parseRow(row)
}

func (c *Client) GetChildren(inode int64) ([]common.Metadata, error) {
	query, err := c.db.Prepare("SELECT * FROM files WHERE parent=?")
	if err != nil {
		return nil, fmt.Errorf("couldn't prepare statement: %v", err)
	}

	row, err := query.Query(inode)
	if err != nil {
		return nil, fmt.Errorf("there is an error in query: %v", err)
	}
	defer row.Close()

	mdList := []common.Metadata{}
	for row.Next() {
		md, err := c.parseRow(row)
		if err != nil {
			return nil, err
		}

		mdList = append(mdList, *md)
	}

	return mdList, nil
}

func (c *Client) parseRow(row *sql.Rows) (*common.Metadata, error) {
	md := &common.Metadata{}

	err := row.Scan(&md.Inode, &md.Name, &md.URL,
		&md.Size, &md.Mode, &md.Parent, &md.Type)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse row: %v", err)
	}

	return md, nil
}
