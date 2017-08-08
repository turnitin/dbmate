package dbmate

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func postgresTestURL(t *testing.T) *url.URL {
	u, err := url.Parse("postgres://postgres:postgres@postgres/dbmate?sslmode=disable")
	require.Nil(t, err)

	return u
}

func prepTestPostgresDB(t *testing.T) *sql.DB {
	drv := PostgresDriver{}
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// connect database
	db, err := sql.Open("postgres", u.String())
	require.Nil(t, err)

	return db
}

func TestPostgresCreateDropDatabase(t *testing.T) {
	drv := PostgresDriver{}
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := sql.Open("postgres", u.String())
		require.Nil(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.Nil(t, err)
	}()

	// drop the database
	err = drv.DropDatabase(u)
	require.Nil(t, err)

	// check that database no longer exists
	func() {
		db, err := sql.Open("postgres", u.String())
		require.Nil(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.NotNil(t, err)
		require.Equal(t, "pq: database \"dbmate\" does not exist", err.Error())
	}()
}

func TestPostgresDatabaseExists(t *testing.T) {
	drv := PostgresDriver{}
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists(u)
	require.Nil(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists(u)
	require.Nil(t, err)
	require.Equal(t, true, exists)
}

func TestPostgresDatabaseExists_Error(t *testing.T) {
	drv := PostgresDriver{}
	u := postgresTestURL(t)
	u.User = url.User("invalid")

	exists, err := drv.DatabaseExists(u)
	require.Equal(t, "pq: role \"invalid\" does not exist", err.Error())
	require.Equal(t, false, exists)
}

func TestPostgresCreateMigrationsTable(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	// migrations table should not exist
	count := 0
	err := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Equal(t, "pq: relation \"schema_migrations\" does not exist", err.Error())

	// create table
	err = drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	// migrations table should exist
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)

	// create table should be idempotent
	err = drv.CreateMigrationsTable(db)
	require.Nil(t, err)
}

func TestPostgresSelectMigrations(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc2'), ('abc1'), ('abc3')`)
	require.Nil(t, err)

	migrations, err := drv.SelectMigrations(db, -1, "default")
	require.Nil(t, err)
	require.Equal(t, true, migrations["abc3"])
	require.Equal(t, true, migrations["abc1"])
	require.Equal(t, true, migrations["abc2"])

	// test limit param
	migrations, err = drv.SelectMigrations(db, 1, "default")
	require.Nil(t, err)
	require.Equal(t, true, migrations["abc3"])
	require.Equal(t, false, migrations["abc1"])
	require.Equal(t, false, migrations["abc2"])

	// test different project
	_, err = db.Exec(`insert into schema_migrations (version, project)
		values ('bcd1', 'app2')`)
	require.Nil(t, err)
	migrations, err = drv.SelectMigrations(db, -1, "app2")
	require.Nil(t, err)
	require.Equal(t, true, migrations["bcd1"])
	require.Equal(t, false, migrations["abc1"])
	require.Equal(t, false, migrations["abc2"])
	require.Equal(t, false, migrations["abc3"])
}

func TestPostgresInsertMigration(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1", "default")
	require.Nil(t, err)

	err = db.QueryRow("select count(*) from schema_migrations where version = 'abc1'").
		Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestPostgresDeleteMigration(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc1'), ('abc2')`)
	require.Nil(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.Nil(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestPostgresLock(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	err := drv.Lock(db)
	require.Nil(t, err)
	defer drv.Unlock(db)

	objectID := 0
	lockType := "lol"
	isGranted := false

	row := db.QueryRow("select objid, mode, granted from pg_locks where objid = 48372615")
	err = row.Scan(&objectID, &lockType, &isGranted)
	require.Nil(t, err)
	require.Equal(t, objectID, 48372615)
	require.Equal(t, lockType, "ExclusiveLock")
	require.Equal(t, isGranted, true)
}
