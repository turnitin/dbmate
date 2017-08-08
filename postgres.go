package dbmate

import (
	"database/sql"
	"fmt"
	"net/url"

	"github.com/lib/pq"
)

// PostgresDriver provides top level database functions
type PostgresDriver struct {
}

// Open creates a new database connection
func (drv PostgresDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("postgres", u.String())
}

func (drv PostgresDriver) openPostgresDB(u *url.URL) (*sql.DB, error) {
	// connect to postgres database
	postgresURL := *u
	postgresURL.Path = "postgres"

	return drv.Open(&postgresURL)
}

// CreateDatabase creates the specified database
func (drv PostgresDriver) CreateDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openPostgresDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("create database %s",
		pq.QuoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv PostgresDriver) DropDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openPostgresDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("drop database if exists %s",
		pq.QuoteIdentifier(name)))

	return err
}

// DatabaseExists determines whether the database exists
func (drv PostgresDriver) DatabaseExists(u *url.URL) (bool, error) {
	name := databaseName(u)

	db, err := drv.openPostgresDB(u)
	if err != nil {
		return false, err
	}
	defer mustClose(db)

	exists := false
	err = db.QueryRow("select true from pg_database where datname = $1", name).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv PostgresDriver) CreateMigrationsTable(db *sql.DB) error {
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
func (drv PostgresDriver) SelectMigrations(db *sql.DB, limit int, project string) (map[string]bool, error) {
	query := "select version from schema_migrations where project = $1 order by version desc"
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
func (drv PostgresDriver) InsertMigration(db Transaction, version string, project string) error {
	_, err := db.Exec("insert into schema_migrations (version, project) values ($1, $2)", version, project)

	return err
}

// DeleteMigration removes a migration record
func (drv PostgresDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec("delete from schema_migrations where version = $1", version)

	return err
}

var lockKey = 48372615

// Lock tries to acquire an advisory lock
func (drv PostgresDriver) Lock(db *sql.DB) error {
	_, err := db.Exec("select pg_advisory_lock($1)", lockKey)
	return err
}

// Unlock releases an advisory lock
func (drv PostgresDriver) Unlock(db *sql.DB) {
	_, err := db.Exec("select pg_advisory_unlock($1)", lockKey)
	if err != nil {
		panic(err)
	}
}
