package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	_ "github.com/glebarez/go-sqlite"

	"github.com/nacorid/x402-feed/internal/server"
)

// Database is a sqlite database
type Database struct {
	db *sql.DB
}

// NewDatabase will open a new database. It will ping the database to ensure it is available and error if not
func NewDatabase(dbPath string) (*Database, error) {
	if dbPath != ":memory:" {
		err := createDbFile(dbPath)
		if err != nil {
			return nil, fmt.Errorf("create db file: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	err = createPostsTable(db)
	if err != nil {
		return nil, fmt.Errorf("creating posts table: %w", err)
	}

	return &Database{db: db}, nil
}

// Close will cleanly stop the database connection
func (d *Database) Close() {
	err := d.db.Close()
	if err != nil {
		slog.Error("failed to close db", "error", err)
	}
}

func createDbFile(dbFilename string) error {
	if _, err := os.Stat(dbFilename); !errors.Is(err, os.ErrNotExist) {
		return nil
	}

	f, err := os.Create(dbFilename)
	if err != nil {
		return fmt.Errorf("create db file : %w", err)
	}
	err = f.Close()
	if err != nil {
		return fmt.Errorf("failed to close DB file: %w", err)
	}
	return nil
}

func createPostsTable(db *sql.DB) error {
	createTableSQL := `CREATE TABLE IF NOT EXISTS posts (
		"id" integer NOT NULL PRIMARY KEY AUTOINCREMENT,
		"postRKey" TEXT,
		"postURI" TEXT,
		"userDID" TEXT NOT NULL DEFAULT '',
		"createdAt" integer NOT NULL,
		UNIQUE(postRKey)
	  );`

	slog.Info("Create posts table...")
	statement, err := db.Prepare(createTableSQL)
	if err != nil {
		return fmt.Errorf("prepare DB statement to create posts table: %w", err)
	}
	_, err = statement.Exec()
	if err != nil {
		return fmt.Errorf("exec sql statement to create posts table: %w", err)
	}
	_ = statement.Close()
	slog.Info("posts table created")

	// Migration for DBs created before userDID existed.
	if err := ensureUserDIDColumn(db); err != nil {
		return err
	}

	return nil
}

func ensureUserDIDColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(posts);`)
	if err != nil {
		return fmt.Errorf("pragma table_info: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan pragma: %w", err)
		}
		if name == "userDID" {
			return nil
		}
	}
	if rows.Err() != nil {
		return fmt.Errorf("iterate pragma rows: %w", rows.Err())
	}
	if _, err := db.Exec(`ALTER TABLE posts ADD COLUMN userDID TEXT NOT NULL DEFAULT '';`); err != nil {
		return fmt.Errorf("add userDID column: %w", err)
	}
	return nil
}

// CreatePost will insert a post into a database
func (d *Database) CreatePost(post server.Post) error {
	sql := `INSERT INTO posts (postRKey, postURI, userDID, createdAt) VALUES (?, ?, ?, ?) ON CONFLICT(postRKey) DO NOTHING;`
	_, err := d.db.Exec(sql, post.RKey, post.PostURI, post.UserDID, post.CreatedAt)
	if err != nil {
		return fmt.Errorf("exec insert post: %w", err)
	}
	return nil
}

// GetFeedPosts return a slice of posts
func (d *Database) GetFeedPosts(cursor, limit int) ([]server.Post, error) {
	sql := `SELECT id, postRKey, postURI, createdAt FROM posts
			WHERE createdAt < ?
			ORDER BY createdAt DESC LIMIT ?;`
	rows, err := d.db.Query(sql, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("run query to get feed posts: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	posts := make([]server.Post, 0)
	for rows.Next() {
		var post server.Post
		if err := rows.Scan(&post.ID, &post.RKey, &post.PostURI, &post.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		posts = append(posts, post)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate rows: %w", rows.Err())
	}

	return posts, nil
}

func (d *Database) DeletePostsFromDIDs(dids []string) error {
	if len(dids) == 0 {
		return nil
	}
	sql := `DELETE FROM posts WHERE userDID IN (?` + strings.Repeat(",?", len(dids)-1) + `);`
	args := make([]interface{}, len(dids))
	for i, did := range dids {
		args[i] = did
	}

	_, err := d.db.Exec(sql, args...)
	if err != nil {
		return fmt.Errorf("exec delete posts: %w", err)
	}
	return nil
}
