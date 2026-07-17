package mariadb

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func connectToDB(username, password, host, port, dbName string) (*sql.DB, error) {
	mysqlDsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?tls=skip-verify", username, password, host, port, dbName)

	db, err := sql.Open("mysql", mysqlDsn)
	if err != nil {
		return nil, fmt.Errorf("could not connect to the mysql server because: %w", err)
	}
	// See "Important settings" section.
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	return db, nil
}

func generateInternalName(dataSourceID, schema, name string) string {
	return fmt.Sprintf("%s/%s@%s", dataSourceID, schema, name)
}
