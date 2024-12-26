package userspace

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	_ "github.com/marcboeker/go-duckdb"
)

const (
	USERSPACE_DB_PATH = "data/db/"
	initSql           = `
    CREATE TABLE IF NOT EXISTS resources (
      rid INTEGER PRIMARY KEY,
      uid INTEGER,
      vid INTEGER,
      gid INTEGER,
      pid INTEGER,
      size INTEGER,
      perms TEXT,
      name TEXT,
      type TEXT,
      created_at DATETIME,
      updated_at DATETIME
    );

    CREATE TABLE IF NOT EXISTS volumes (
      vid INTEGER PRIMARY KEY,
      path TEXT,
      capacity INTEGER,
      usage INTEGER
    );

    CREATE SEQUENCE IF NOT EXISTS seq_resourceid START 1;
    CREATE SEQUENCE IF NOT EXISTS seq_volumeid START 1; 
    
    `
)

type DBHandler struct {
	db     *sql.DB
	DBName string
}

func (m *DBHandler) getConn() (*sql.DB, error) {
	if m.db == nil {
		db, err := sql.Open("duckdb", USERSPACE_DB_PATH+m.DBName)
		if err != nil {
			return nil, err
		}
		m.db = db
	}
	return m.db, nil
}

func (m *DBHandler) Close() {
	if m.db != nil {
		m.db.Close()
	}
}

func (m *DBHandler) Init() {
	log.Printf("Initializing %v database", m.DBName)
	_, err := os.Stat("data")
	if err != nil {
		err = os.Mkdir("data", 0o700)
		if err != nil {
			log.Fatalf("failed to make new directory, destructive: %v", err)
		}
	}

	_, err = os.Stat("data/db")
	if err != nil {
		err = os.Mkdir("data/db", 0o700)
		if err != nil {
			log.Fatalf("failed to make new directory, destructive: %v", err)
		}
	}

	db, err := m.getConn()
	if err != nil {
		log.Fatalf("couldn't get db connection, destructive: %v", err)
	}

	_, err = db.Exec(initSql)
	if err != nil {
		log.Fatalf("failed to init db, destrcutive: %v", err)
	}
}

func (m *DBHandler) InsertResources(resources []Resource) error {
	db, err := m.getConn()
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	placeholder := strings.Repeat("(nextval('seq_resourceid'), ?, ?, ?, ?, ?, ?, ?, ?, ?),", len(resources))
	query := fmt.Sprintf(`
    INSERT INTO 
      resources (rid, uid, vid, name, type, pid, oid, gid, perms, size, created_at, updated_at) 
    VALUES %s`, placeholder[:len(placeholder)-1])
	query += `;`

	log.Print(query)

	stmt, err := tx.Prepare(query)
	if err != nil {
		log.Printf("error preparing transaction: %v", err)
		return err
	}
	defer stmt.Close()

	for _, r := range resources {
		_, err = stmt.Exec(r.FieldsNoId()...)
		if err != nil {
			log.Printf("error executing transaction: %v", err)
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		log.Printf("failed to commit transaction: %v", err)
		return err
	}

	return nil
}

func (m *DBHandler) GetAllResources() ([]Resource, error) {
	db, err := m.getConn()
	if err != nil {
		log.Printf("error getting db connection: %v", err)
		return nil, err
	}

	rows, err := db.Query("SELECT * FROM resources")
	if err != nil {
		log.Printf("error querying db: %v", err)
		return nil, err
	}
	defer rows.Close()

	var resources []Resource
	for rows.Next() {
		var r Resource
		err = rows.Scan(r.PtrFields()...)
		if err != nil {
			log.Printf("error scanning row: %v", err)
			return nil, err
		}
		log.Printf("resource: %v", r)
		resources = append(resources, r)
	}

	return resources, nil
}

func (m *DBHandler) GetResourceByIds(rids []int) ([]Resource, error) {
	db, err := m.getConn()
	if err != nil {
		log.Printf("error getting db connection: %v", err)
		return nil, err
	}

	rows, err := db.Query("SELECT * FROM resources WHERE rid IN (?)", rids)
	if err != nil {
		log.Printf("error querying db: %v", err)
		return nil, err
	}

	var resources []Resource
	for rows.Next() {
		var r Resource
		err = rows.Scan(r.PtrFields()...)
		if err != nil {
			log.Printf("error scanning row: %v", err)
			return nil, err
		}
		resources = append(resources, r)
	}
	return resources, nil
}

func (m *DBHandler) DeleteResourcesByIds(rids []int) error {
	db, err := m.getConn()
	if err != nil {
		log.Printf("error getting db connection: %v", err)
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("error starting transaction: %v", err)
		return err
	}

	res, err := tx.Exec("DELETE FROM resources WHERE rid IN (?)", rids)
	if err != nil {
		log.Printf("failed to execute query: %v", err)
		return err
	}

	rAff, err := res.RowsAffected()
	if err != nil {
		log.Printf("failed to get rows affected: %v", err)
		return err
	}

	err = tx.Commit()
	if err != nil {
		log.Printf("failed to commit transaction: %v", err)
		return err
	}
	log.Printf("deleted %v rows", rAff)

	return nil
}

func (m *DBHandler) UpdateResourceById(rid int, r Resource) error {
	db, err := m.getConn()
	if err != nil {
		log.Printf("error getting db connection: %v", err)
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("error starting transaction: %v", err)
		return err
	}

	// we should check which fields are empty and not update those...

	var setClauses []string
	var params []interface{}

	val := reflect.ValueOf(r)
	typ := reflect.TypeOf(r)

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldName := typ.Field(i).Name

		if isEmpty(field) {
			continue
		}

		columnName := toSnakeCase(fieldName)
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", columnName))
		params = append(params, field.Interface())
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	params = append(params, rid)

	query := fmt.Sprintf(`
    UPDATE 
      resources 
    SET 
      %s 
    WHERE 
      rid = ?
  `, strings.Join(setClauses, ", "))

	stmt, err := tx.Prepare(query)
	if err != nil {
		log.Printf("error preparing transaction: %v", err)
		return err
	}

	stmt.Exec()

	return nil
}

func (m *DBHandler) InsertVolumes(volumes []Volume) error {
	db, err := m.getConn()
	if err != nil {
		log.Printf("error getting db connection: %v", err)
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("error starting transaction: %v", err)
		return err
	}

	placeholder := strings.Repeat("(nextval('seq_volumeid'), ?, ?, ?),", len(volumes))
	query := fmt.Sprintf(`
    INSERT INTO 
      volumes (vid, path, capacity, usage)
    VALUES %s`, placeholder[:len(placeholder)-1])

	stmt, err := tx.Prepare(query)
	if err != nil {
		log.Printf("error preparing transaction: %v", err)
		return err
	}
	defer stmt.Close()

	for _, v := range volumes {
		_, err = stmt.Exec(v.Path, v.Capacity, v.Usage)
		if err != nil {
			log.Printf("error executing transaction: %v", err)
			tx.Rollback()
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		log.Printf("failed to commit transaction: %v", err)
		return err
	}

	return nil
}

// Helper function to determine if a value is empty
func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	default:
		return v.IsZero() // General case for other types
	}
}