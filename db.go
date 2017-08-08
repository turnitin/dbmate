package dbmate

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// DefaultMigrationsDir specifies default directory to find migration files
var DefaultMigrationsDir = "./db/migrations"

// DefaultProject specifies the default name to associate with the migrations
var DefaultProject = "default"

// DB allows dbmate actions to be performed on a specified database
type DB struct {
	DatabaseURL   *url.URL
	MigrationsDir string
	Project       string
}

// NewDB initializes a new dbmate database
func NewDB(databaseURL *url.URL) *DB {
	return &DB{
		DatabaseURL:   databaseURL,
		MigrationsDir: DefaultMigrationsDir,
		Project:       DefaultProject,
	}
}

// GetDriver loads the required database driver
func (db *DB) GetDriver() (Driver, error) {
	return GetDriver(db.DatabaseURL.Scheme)
}

// Up creates the database (if necessary) and runs migrations
func (db *DB) Up(lockTimeoutSecs int) error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	// create database if it does not already exist
	// skip this step if we cannot determine status
	// (e.g. user does not have list database permission)
	exists, err := drv.DatabaseExists(db.DatabaseURL)
	if err == nil && !exists {
		if err := drv.CreateDatabase(db.DatabaseURL); err != nil {
			return err
		}
	}

	// migrate
	return db.Migrate(lockTimeoutSecs)
}

// Create creates the current database
func (db *DB) Create() error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	return drv.CreateDatabase(db.DatabaseURL)
}

// Drop drops the current database (if it exists)
func (db *DB) Drop() error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	return drv.DropDatabase(db.DatabaseURL)
}

const migrationTemplate = "-- migrate:up\n\n\n-- migrate:down\n\n"

// New creates a new migration file
func (db *DB) New(name string) error {
	// new migration name
	timestamp := time.Now().UTC().Format("20060102150405")
	if name == "" {
		return fmt.Errorf("please specify a name for the new migration")
	}
	name = fmt.Sprintf("%s_%s.sql", timestamp, name)

	// create migrations dir if missing
	if err := os.MkdirAll(db.MigrationsDir, 0755); err != nil {
		return fmt.Errorf("unable to create directory `%s`", db.MigrationsDir)
	}

	// check file does not already exist
	path := filepath.Join(db.MigrationsDir, name)
	fmt.Printf("Creating migration: %s\n", path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("file already exists")
	}

	// write new migration
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	defer mustClose(file)
	_, err = file.WriteString(migrationTemplate)
	if err != nil {
		return err
	}

	return nil
}

func doTransaction(db *sql.DB, txFunc func(Transaction) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if err := txFunc(tx); err != nil {
		if err1 := tx.Rollback(); err1 != nil {
			return err1
		}

		return err
	}

	return tx.Commit()
}

func (db *DB) openDatabaseForMigration() (Driver, *sql.DB, error) {
	drv, err := db.GetDriver()
	if err != nil {
		return nil, nil, err
	}

	sqlDB, err := drv.Open(db.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}

	if err := drv.CreateMigrationsTable(sqlDB); err != nil {
		mustClose(sqlDB)
		return nil, nil, err
	}

	return drv, sqlDB, nil
}

// Migrate migrates database to the latest version
func (db *DB) Migrate(lockTimeoutSecs int) error {
	re := regexp.MustCompile(`^\d.*\.sql$`)
	files, err := findMigrationFiles(db.MigrationsDir, re)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no migration files found")
	}

	drv, sqlDB, err := db.openDatabaseForMigration()
	if err != nil {
		return err
	}
	defer mustClose(sqlDB)

	return RunInLock(drv, sqlDB, lockTimeoutSecs, func(driver Driver, sqlDB *sql.DB) error {
		alreadyApplied, err := driver.SelectMigrations(sqlDB, -1, db.Project)
		if err != nil {
			return err
		}

		for _, filename := range files {
			ver := migrationVersion(filename)
			if ok := alreadyApplied[ver]; ok {
				continue
			}
			fmt.Printf("Applying: %s\n", filename)
			migration, err := parseMigration(filepath.Join(db.MigrationsDir, filename))
			if err != nil {
				return err
			}

			// begin transaction
			err = doTransaction(sqlDB, func(tx Transaction) error {
				// run actual migration
				if _, err := tx.Exec(migration["up"]); err != nil {
					return err
				}

				// record migration
				if err := drv.InsertMigration(tx, ver, db.Project); err != nil {
					return err
				}

				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func findMigrationFiles(dir string, re *regexp.Regexp) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("could not find migrations directory `%s`", dir)
	}

	matches := []string{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !re.MatchString(name) {
			continue
		}

		matches = append(matches, name)
	}

	sort.Strings(matches)

	return matches, nil
}

func findMigrationFile(dir string, ver string) (string, error) {
	if ver == "" {
		panic("migration version is required")
	}

	ver = regexp.QuoteMeta(ver)
	re := regexp.MustCompile(fmt.Sprintf(`^%s.*\.sql$`, ver))

	files, err := findMigrationFiles(dir, re)
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("can't find migration file: %s*.sql", ver)
	}

	return files[0], nil
}

func migrationVersion(filename string) string {
	return regexp.MustCompile(`^\d+`).FindString(filename)
}

// parseMigration reads a migration file into a map with up/down keys
// implementation is similar to regexp.Split()
func parseMigration(path string) (map[string]string, error) {
	// read migration file into string
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	contents := string(data)

	// split string on our trigger comment
	separatorRegexp := regexp.MustCompile(`(?m)^-- migrate:(.*)$`)
	matches := separatorRegexp.FindAllStringSubmatchIndex(contents, -1)

	migrations := map[string]string{}
	direction := ""
	beg := 0
	end := 0

	for _, match := range matches {
		end = match[0]
		if direction != "" {
			// write previous direction to output map
			migrations[direction] = contents[beg:end]
		}

		// each match records the start of a new direction
		direction = contents[match[2]:match[3]]
		beg = match[1]
	}

	// write final direction to output map
	migrations[direction] = contents[beg:]

	return migrations, nil
}

// Rollback rolls back the most recent migration
func (db *DB) Rollback() error {
	drv, sqlDB, err := db.openDatabaseForMigration()
	if err != nil {
		return err
	}
	defer mustClose(sqlDB)

	applied, err := drv.SelectMigrations(sqlDB, 1, db.Project)
	if err != nil {
		return err
	}

	// grab most recent applied migration (applied has len=1)
	latest := ""
	for ver := range applied {
		latest = ver
	}
	if latest == "" {
		return fmt.Errorf("can't rollback: no migrations have been applied")
	}

	filename, err := findMigrationFile(db.MigrationsDir, latest)
	if err != nil {
		return err
	}

	fmt.Printf("Rolling back: %s\n", filename)

	migration, err := parseMigration(filepath.Join(db.MigrationsDir, filename))
	if err != nil {
		return err
	}

	// begin transaction
	err = doTransaction(sqlDB, func(tx Transaction) error {
		// rollback migration
		if _, err := tx.Exec(migration["down"]); err != nil {
			return err
		}

		// remove migration record
		if err := drv.DeleteMigration(tx, latest); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
