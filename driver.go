package dbmate

import (
	"database/sql"
	"fmt"
	"net/url"
	"time"
)

// Driver provides top level database functions
type Driver interface {
	Open(*url.URL) (*sql.DB, error)
	DatabaseExists(*url.URL) (bool, error)
	CreateDatabase(*url.URL) error
	DropDatabase(*url.URL) error
	CreateMigrationsTable(*sql.DB) error
	SelectMigrations(*sql.DB, int, string) (map[string]bool, error)
	InsertMigration(Transaction, string, string) error
	DeleteMigration(Transaction, string) error
	Lock(*sql.DB) error
	Unlock(*sql.DB)
}

// Transaction can represent a database or open transaction
type Transaction interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// RunInLock will execute a function in the context of the driver's advisory lock
func RunInLock(driver Driver, sqlDB *sql.DB, timeoutSecs int, lockFunc func(Driver, *sql.DB) error) error {
	lockChan := make(chan string, 1)
	errChan := make(chan error, 1)
	go func() {
		if err := driver.Lock(sqlDB); err != nil {
			errChan <- err
		} else {
			lockChan <- "acquired"
		}
	}()
	select {
	case <-lockChan:
		defer driver.Unlock(sqlDB)
		return lockFunc(driver, sqlDB)
	case err := <-errChan:
		return err
	case <-time.After(time.Second * time.Duration(timeoutSecs)):
		return fmt.Errorf("Timeout waiting for database migration lock (waited %v seconds)", timeoutSecs)
	}
}

// GetDriver loads a database driver by name
func GetDriver(name string) (Driver, error) {
	switch name {
	case "mysql":
		return MySQLDriver{}, nil
	case "postgres", "postgresql":
		return PostgresDriver{}, nil
	case "sqlite", "sqlite3":
		return SQLiteDriver{}, nil
	default:
		return nil, fmt.Errorf("unknown driver: %s", name)
	}
}

// GetDriverOpen is a shortcut for GetDriver(u.Scheme).Open(u)
func GetDriverOpen(u *url.URL) (*sql.DB, error) {
	drv, err := GetDriver(u.Scheme)
	if err != nil {
		return nil, err
	}

	return drv.Open(u)
}
