package dbmate

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"

	_ "github.com/mattn/go-sqlite3" // sqlite driver for database/sql
)

// SQLiteDriver provides top level database functions
type SQLiteDriver struct {
}

func sqlitePath(u *url.URL) string {
	// strip one leading slash
	// absolute URLs can be specified as sqlite:////tmp/foo.sqlite3
	str := regexp.MustCompile("^/").ReplaceAllString(u.Path, "")

	return str
}

// Open creates a new database connection
func (drv SQLiteDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("sqlite3", sqlitePath(u))
}

// CreateDatabase creates the specified database
func (drv SQLiteDriver) CreateDatabase(u *url.URL) error {
	fmt.Printf("Creating: %s\n", sqlitePath(u))

	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	return db.Ping()
}

// DropDatabase drops the specified database (if it exists)
func (drv SQLiteDriver) DropDatabase(u *url.URL) error {
	path := sqlitePath(u)
	fmt.Printf("Dropping: %s\n", path)

	exists, err := drv.DatabaseExists(u)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	return os.Remove(path)
}

// DatabaseExists determines whether the database exists
func (drv SQLiteDriver) DatabaseExists(u *url.URL) (bool, error) {
	_, err := os.Stat(sqlitePath(u))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateMigrationsTable creates the schema_migrations table
func (drv SQLiteDriver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`create table if not exists schema_migrations (
		version varchar(255) primary key)`)
	if err != nil {
		return err
	}

	// Add the project column if it doesn't already exist.
	_, err = db.Exec(`select project from schema_migrations limit 1`)
	if err != nil {
		_, err = db.Exec(`alter table schema_migrations
			add column project varchar(255) default 'default'`)
	}

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv SQLiteDriver) SelectMigrations(db *sql.DB, limit int, project string) (map[string]bool, error) {
	query := "select version from schema_migrations where project = ? order by version desc"
	if limit >= 0 {
		query = fmt.Sprintf("%s limit %d", query, limit)
	}
	rows, err := db.Query(query, project)
	if err != nil {
		return nil, err
	}

	defer mustClose(rows)

	migrations := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}

		migrations[version] = true
	}

	return migrations, nil
}

// InsertMigration adds a new migration record
func (drv SQLiteDriver) InsertMigration(db Transaction, version string, project string) error {
	_, err := db.Exec("insert into schema_migrations (version, project) values (?, ?)", version, project)

	return err
}

// DeleteMigration removes a migration record
func (drv SQLiteDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec("delete from schema_migrations where version = ?", version)

	return err
}

// Lock locks the database so no other migrations can be run, no-op in SQLite
func (drv SQLiteDriver) Lock(db *sql.DB) error {
	return nil
}

// Unlock removes a database lock so other migrations can be run, no-op in SQLite
func (drv SQLiteDriver) Unlock(db *sql.DB) {
	// no-op
}
