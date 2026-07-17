package mariadb

import (
	"fmt"

	"github.com/coordimap/agent/pkg/domain/database"
	"github.com/coordimap/agent/pkg/domain/mariadb"
)

func (mariaCrawler *mariadbCrawler) GetTableNames(schemaName string) ([]string, error) {
	foundTableNames := []string{}

	query := "SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE'"

	rows, err := mariaCrawler.dbConn.Query(query, schemaName)
	if err != nil {
		return foundTableNames, fmt.Errorf("could not retrieve the table names because %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return foundTableNames, err
		}

		foundTableNames = append(foundTableNames, name)
	}

	return foundTableNames, nil
}

func (mariaCrawler *mariadbCrawler) GetTableData(schemaName, tableName string) (database.Table, error) {
	table := database.Table{
		Name:        tableName,
		Columns:     []database.Column{},
		Constraints: []database.Constraint{},
		Schema:      schemaName,
		Indexes:     []string{},
	}

	// get table columns
	columns, errColumns := mariaCrawler.getTableColumns(schemaName, tableName)
	if errColumns == nil {
		table.Columns = append(table.Columns, columns...)
	}

	// get constraints
	constraints, errConstraints := mariaCrawler.getTableConstraints(schemaName, tableName)
	if errConstraints == nil {
		table.Constraints = append(table.Constraints, constraints...)
	}

	return table, nil
}

func (mariaCrawler *mariadbCrawler) getTableColumns(schemaName, tableName string) ([]database.Column, error) {
	columns := []database.Column{}
	sqlQuery := `
		SELECT column_name, column_type, ordinal_position -- column_default, is_nullable, column_comment, extra, generation_expression
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`

	rows, err := mariaCrawler.dbConn.Query(sqlQuery, schemaName, tableName)
	if err != nil {
		return columns, fmt.Errorf("could not retrieve the table columns because %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var column database.Column
		if err := rows.Scan(&column.Name, &column.Type, &column.Position); err != nil {
			return columns, err
		}

		column.Table = generateInternalName(mariaCrawler.scopeID, schemaName, tableName)

		columns = append(columns, column)
	}

	return columns, nil
}

func (mariaCrawler *mariadbCrawler) getTableConstraints(schemaName, tableName string) ([]database.Constraint, error) {
	constraints := []database.Constraint{}

	// get primary key
	sqlPrimaryKeyQuery := `
	SELECT kcu.COLUMN_NAME, kcu.ORDINAL_POSITION, c.data_type
	FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE as kcu,
				information_schema.columns as c
	WHERE kcu.table_name = c.table_name and
				kcu.column_name = c.column_name and
        kcu.TABLE_SCHEMA = ? and
        kcu.CONSTRAINT_NAME='PRIMARY' and
				c.column_key = 'PRI' and
				kcu.TABLE_NAME = ?;
	`
	primaryKeyRows, errPrimaryKeyRows := mariaCrawler.dbConn.Query(sqlPrimaryKeyQuery, schemaName, tableName)
	if errPrimaryKeyRows != nil {
		return constraints, fmt.Errorf("could not retrieve the table columns because %w", errPrimaryKeyRows)
	}

	pkConstraint := database.Constraint{
		Name:         "PRIMARY KEY",
		Type:         mariadb.MARIADB_CONSTRAINT_PK,
		Sources:      []database.Column{},
		Destinations: []database.Column{},
	}

	for primaryKeyRows.Next() {
		var col database.Column

		if err := primaryKeyRows.Scan(&col.Name, &col.Position, &col.Type); err != nil {
			continue
		}

		col.Table = mariaCrawler.Host + "/" + schemaName + "@" + tableName

		pkConstraint.Sources = append(pkConstraint.Sources, col)
	}

	if len(pkConstraint.Sources) > 0 {
		constraints = append(constraints, pkConstraint)
	}

	// get constraints names except for the primary keys(uniq, fk)
	sqlConstraintsNames := `
	select kcu.constraint_name
	FROM information_schema.key_column_usage AS kcu,
					information_schema.columns as c
	WHERE kcu.table_name = c.table_name AND
					kcu.table_schema = c.table_schema AND
					kcu.table_schema = ? AND
					kcu.table_name = ? AND
					c.column_key != 'PRI' AND kcu.constraint_name != 'PRIMARY'
    ;
	`
	constraintsNamesRows, errConstraintsNameRows := mariaCrawler.dbConn.Query(sqlConstraintsNames, schemaName, tableName)
	if errConstraintsNameRows != nil {

	}

	constraintNames := []string{}

	for constraintsNamesRows.Next() {
		var constraintName string

		if err := constraintsNamesRows.Scan(&constraintName); err != nil {
			continue
		}

		constraintNames = append(constraintNames, constraintName)
	}

	for _, constraintName := range constraintNames {
		constraintColumnsQuery := `
		SELECT kcu.TABLE_NAME, kcu.COLUMN_NAME, kcu.ORDINAL_POSITION, c1.data_type, c.data_type, c.ordinal_position, c.column_name,
				(CASE WHEN kcu.referenced_table_name IS NOT NULL THEN kcu.referenced_table_name
							WHEN kcu.referenced_table_name IS NULL THEN c.table_name
				END) AS referenced_table,
				(CASE WHEN kcu.referenced_table_name IS NOT NULL THEN 'mariadb.foreign_key'
						WHEN c.column_key = 'PRI' AND kcu.constraint_name = 'PRIMARY' THEN 'mariadb.primary_key'
						WHEN c.column_key = 'PRI' AND kcu.constraint_name != 'PRIMARY' THEN 'mariadb.unique'
						WHEN c.column_key = 'UNI' THEN 'mariadb.unique'
						WHEN c.column_key = 'MUL' THEN 'mariadb.unique'
						ELSE 'UNKNOWN'
				END) AS costraint_type
		FROM (INFORMATION_SCHEMA.KEY_COLUMN_USAGE as kcu INNER JOIN information_schema.columns as c1 ON kcu.table_name = c1.table_name AND kcu.column_name = c1.column_name)  LEFT JOIN
            information_schema.columns as c
			ON kcu.referenced_table_name = c.table_name and
            kcu.referenced_column_name = c.column_name
		WHERE kcu.TABLE_SCHEMA = ? and kcu.constraint_name = ?;
		`

		constraint := database.Constraint{
			Name:         constraintName,
			Type:         "",
			Sources:      []database.Column{},
			Destinations: []database.Column{},
		}

		constraintColumnsRows, err := mariaCrawler.dbConn.Query(constraintColumnsQuery, schemaName, constraintName)
		if err != nil {
			continue
		}

		for constraintColumnsRows.Next() {
			var constraintType string
			var columnFrom, columnTo database.Column

			if err := constraintColumnsRows.Scan(&columnFrom.Table, &columnFrom.Name, &columnFrom.Position, &columnFrom.Type, &columnTo.Type, &columnTo.Position, &columnTo.Name, &columnTo.Table, &constraintType); err != nil {
				continue
			}

			columnFrom.Table = generateInternalName(mariaCrawler.scopeID, schemaName, columnFrom.Table)

			if constraintType != "UNKNOWN" {
				constraint.Type = constraintType
			}

			switch constraint.Type {
			case mariadb.MARIADB_CONSTRAINT_FK:
				columnTo.Table = generateInternalName(mariaCrawler.scopeID, schemaName, columnTo.Table)
				constraint.Destinations = append(constraint.Destinations, columnTo)
			}
		}

		constraints = append(constraints, constraint)

	}

	return constraints, nil
}

func (mariaCrawler *mariadbCrawler) getTableIndexes(schemaName, tableName string) ([]database.Index, error) {
	allIndexes := []database.Index{}

	indexesNamesQuery := `
	SELECT DISTINCT
			INDEX_NAME
	FROM INFORMATION_SCHEMA.STATISTICS
	WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?;
	`
	indexesNamesRows, errIndexesNamesRows := mariaCrawler.dbConn.Query(indexesNamesQuery, schemaName, tableName)
	if errIndexesNamesRows != nil {
		return allIndexes, fmt.Errorf("could not retrieve the table columns because %w", errIndexesNamesRows)
	}

	for indexesNamesRows.Next() {
		var indexName string

		if err := indexesNamesRows.Scan(&indexName); err != nil {
			continue
		}

		// get index data
		index := database.Index{
			Name:    indexName,
			Columns: []database.Column{},
			Table:   generateInternalName(mariaCrawler.scopeID, schemaName, tableName),
			Schema:  schemaName,
		}

		indexColumnsQuery := `
		SELECT s.column_name, s.seq_in_index, c.data_type
		FROM information_schema.statistics AS s,
				information_schema.COLUMNS AS c
		WHERE s.TABLE_schema = c.table_schema AND
				s.table_name = c.table_name AND
				s.column_name = c.column_name AND
				s.table_schema = ? AND
				s.table_name = ? AND
				s.index_name = ?;
		`
		indexColumnsRows, errIndexColumnsRows := mariaCrawler.dbConn.Query(indexColumnsQuery, schemaName, tableName, indexName)
		if errIndexColumnsRows != nil {
			continue
		}

		for indexColumnsRows.Next() {
			var indexColumn database.Column

			if err := indexColumnsRows.Scan(&indexColumn.Name, &indexColumn.Position, &indexColumn.Type); err != nil {
				continue
			}

			indexColumn.Table = generateInternalName(mariaCrawler.scopeID, schemaName, tableName)

			index.Columns = append(index.Columns, indexColumn)
		}

		allIndexes = append(allIndexes, index)
	}

	return allIndexes, nil

}
