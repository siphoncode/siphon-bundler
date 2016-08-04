package bundler

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	// Loads the PostgreSQL driver for database/sql.
	_ "github.com/lib/pq"
)

const filesTable = "files"

// OpenDB returns a configured connection to the postgres database.
func OpenDB() *sql.DB {
	url := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		os.Getenv("POSTGRES_BUNDLER_ENV_POSTGRES_USER"),
		os.Getenv("POSTGRES_BUNDLER_ENV_POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_BUNDLER_PORT_5432_TCP_ADDR"),
		os.Getenv("POSTGRES_BUNDLER_ENV_POSTGRES_DB"))
	db, err := sql.Open("postgres", url)
	if err != nil {
		log.Fatalf("Error opening DB connection: %v", err)
	}
	return db
}

// Returns SQL suitable for filtering on the given submission ID.
func subClause(submissionID string) string {
	if submissionID != "" {
		return fmt.Sprintf("submission_id = '%s'", submissionID)
	}
	return "(submission_id is null or submission_id='')"
}

// AddFile adds a new file row associated with the given `appID`. Leave the
// `submissionID` empty to store it as a development file.
func AddFile(db *sql.DB, appID string, submissionID string, name string,
	hash string) error {
	rows, err := db.Query(
		fmt.Sprintf("INSERT INTO %s (submission_id, app_id, name, hash) "+
			"VALUES ($1, $2, $3, $4)", filesTable),
		submissionID, appID, name, hash)
	rows.Close()
	if err != nil {
		log.Printf("AddFile() error: %v", err)
		return fmt.Errorf("Failed to save file: %s", name)
	}
	return nil
}

// UpdateFile updates the hash stored for an existing file row.
func UpdateFile(db *sql.DB, appID string, submissionID string, name string,
	hash string) error {
	rows, err := db.Query(
		fmt.Sprintf("UPDATE %s SET hash = $1 WHERE app_id = $2 AND %s "+
			"AND name = $3", filesTable, subClause(submissionID)),
		hash, appID, name)
	rows.Close()
	if err != nil {
		log.Printf("UpdateFile() error: %v", err)
		return fmt.Errorf("Failed to update file: %s", name)
	}
	return nil
}

// DeleteFile removes the row for this file name.
func DeleteFile(db *sql.DB, appID string, submissionID string,
	name string) error {
	rows, err := db.Query(
		fmt.Sprintf("DELETE FROM %s WHERE app_id = $1 AND %s AND name = $2",
			filesTable, subClause(submissionID)), appID, name)
	rows.Close()
	if err != nil {
		log.Printf("DeleteFile() error: %v", err)
		return fmt.Errorf("Failed to delete file: %s", name)
	}
	return nil
}

// GetFile returns the hash for an individual file (note: `name` is used
// as an SQL LIKE clause, but this function will return an error if multiple
// files are returned).
func GetFile(db *sql.DB, appID string, submissionID string, name string) (
	hash string, err error) {
	files, err := GetFilteredFiles(db, appID, submissionID, name)
	if err != nil {
		return "", err
	}
	hash, ok := files[name]
	if !ok {
		return "", errors.New("[GetFile] not found: " + name)
	}
	return hash, nil
}

// GetFiles returns every stored file (name -> SHA-256 hash) for the given
// App ID and Submission ID.
func GetFiles(db *sql.DB, appID string, submissionID string) (
	files map[string]string, err error) {
	return GetFilteredFiles(db, appID, submissionID, "")
}

// GetFilteredFiles returns our currently stored files (name -> SHA-256 hash)
// for the given App ID and Submission ID, but filters the name on SQL LIKE.
func GetFilteredFiles(db *sql.DB, appID string, submissionID string,
	like string) (files map[string]string, err error) {
	// An empty `like` means we match any name.
	if like == "" {
		like = "%"
	}

	rows, err := db.Query(
		fmt.Sprintf("SELECT name, hash FROM %s WHERE app_id = $1 AND %s "+
			"AND name LIKE $2", filesTable, subClause(submissionID)),
		appID, like)
	if err != nil {
		log.Printf("GetFiles() query error: %v", err)
		return nil, fmt.Errorf("Failed to retrieve current app files.")
	}
	defer rows.Close()

	// Add each row as name->hash in our map
	files = map[string]string{}
	var name string
	var hash string
	for rows.Next() {
		err = rows.Scan(&name, &hash)
		if err != nil {
			log.Printf("GetFiles() scan error: %v", err)
			return nil, fmt.Errorf("Failed to retrieve current app files.")
		}
		files[name] = hash
	}
	return files, nil
}

// Like GetFilteredFiles but takes a slice of files to filter
func GetSliceFilteredFiles(db *sql.DB, appID string, submissionID string,
	like []string) (files map[string]string, err error) {
	var likeStr string
	// An empty `like` means we match any name.
	if like == nil || len(like) == 0 {
		like = []string{"%"}
	} else {
		sqlVars := []string{}
		// Note: postgres-specific query
		for i := 0; i < len(like); i++ {
			sqlVars = append(sqlVars, fmt.Sprintf("$%d", i+2))
		}
		likeStr = fmt.Sprintf("ANY(ARRAY[%s])", strings.Join(sqlVars, ", "))
	}

	args := []interface{}{appID}
	for i := 0; i < len(like); i++ {
		args = append(args, like[i])
	}

	rows, err := db.Query(
		fmt.Sprintf("SELECT name, hash FROM %s WHERE app_id = $1 AND %s "+
			"AND name LIKE %s", filesTable, subClause(submissionID), likeStr),
		args...)
	if err != nil {
		log.Printf("GetFiles() query error: %v", err)
		return nil, fmt.Errorf("Failed to retrieve current app files.")
	}
	defer rows.Close()

	// Add each row as name->hash in our map
	files = map[string]string{}
	var name string
	var hash string
	for rows.Next() {
		err = rows.Scan(&name, &hash)
		if err != nil {
			log.Printf("GetFiles() scan error: %v", err)
			return nil, fmt.Errorf("Failed to retrieve current app files.")
		}
		files[name] = hash
	}
	return files, nil
}

func resourceExists(db *sql.DB, name string, resourceID string) (bool, error) {
	rows, err := db.Query(
		fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = $1", filesTable, name),
		resourceID)
	if err != nil {
		log.Printf("resourceExists() query error: %v", err)
		return false, fmt.Errorf("Failed to check for '%s' existence: %s",
			name, resourceID)
	}
	defer rows.Close()
	rows.Next()
	var count int
	if err := rows.Scan(&count); err != nil {
		log.Printf("resourceExists() scan error: %v", err)
		return false, fmt.Errorf("Failed to check for '%s' existence: %s",
			name, resourceID)
	}
	return count > 0, nil
}

// AppExists returns true if one-or-more files exist for a given app ID.
func AppExists(db *sql.DB, appID string) (bool, error) {
	return resourceExists(db, "app_id", appID)
}

// SubmissionExists returns true if one-or-more files exist for a given
// submission ID.
func SubmissionExists(db *sql.DB, submissionID string) (bool, error) {
	return resourceExists(db, "submission_id", submissionID)
}

// MakeSnapshot takes an app ID and makes a copy of the rows with the
// column "submission_id" set to the one given.
func MakeSnapshot(db *sql.DB, appID string, submissionID string) error {
	files, err := GetFiles(db, appID, "") // fetch current dev file rows
	if err != nil {
		log.Printf("[MakeSnapshot() get error]: %s, %v", appID, err)
		return errors.New("Problem retrieving files for the snapshot")
	}
	for name, hash := range files {
		err := AddFile(db, appID, submissionID, name, hash)
		if err != nil {
			log.Printf("[MakeSnapshot() add error]: %s, %s, %s, %s, %v",
				appID, submissionID, name, hash, err)
			return errors.New("Problem retrieving files for the snapshot")
		}
	}
	return nil // success
}

// CreateTables lazily creates the required tables in the bundler DB.
func CreateTables() {
	db := OpenDB()
	defer db.Close()

	// Lazily create our metadata table
	rows, err := db.Query(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id bigserial PRIMARY KEY,
			app_id varchar(64) NOT NULL,
			submission_id varchar(64) DEFAULT null,
			name text NOT NULL, /* flat path e.g. "assets/my-image.png" */
			hash text NOT NULL /* SHA-256 */
		)
	`, filesTable))
	rows.Close()
	if err != nil {
		log.Fatalf("Error creating table: %v", err)
	}

	// We need to partial unique indexes because submission_id can be null,
	// see here: http://stackoverflow.com/a/8289253
	rows, err = db.Query(fmt.Sprintf(`
		DO $$
		BEGIN
			IF (SELECT to_regclass('files_unique_name_1')) IS NULL THEN
				CREATE UNIQUE INDEX files_unique_name_1 ON %s
				(app_id, submission_id, name) WHERE submission_id IS NOT null;
			END IF;
			IF (SELECT to_regclass('files_unique_name_2')) IS NULL THEN
				CREATE UNIQUE INDEX files_unique_name_2 ON %s
				(app_id, name) WHERE submission_id IS null;
			END IF;
		END $$;
	`, filesTable, filesTable))
	rows.Close()
	if err != nil {
		log.Fatalf("Error creating unique indexes: %v", err)
	}

	// Lazily create our column indices
	indices := map[string]string{
		"files_app_id_index":        "app_id",
		"files_name_index":          "name",
		"files_submission_id_index": "submission_id",
	}
	for indexName, cols := range indices {
		rows, err = db.Query(fmt.Sprintf(`
			DO $$
			BEGIN
				IF (SELECT to_regclass('%s')) IS NULL THEN
					CREATE INDEX %s ON %s(%s);
				END IF;
			END $$;
		`, indexName, indexName, filesTable, cols))
		rows.Close()
		if err != nil {
			log.Fatalf("Error creating index %s: %v", indexName, err)
		}
	}
}
