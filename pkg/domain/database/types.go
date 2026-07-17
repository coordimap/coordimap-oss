package database

type Database struct {
	Name    string   `json:"db_name"`
	Host    string   `json:"db_host"`
	Schemas []string `json:"schemas"`
}

type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Position int    `json:"position"`
	Table    string `json:"table"`
}

type Index struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
	Table   string   `json:"table"`
	Schema  string   `json:"schema"`
}

type Constraint struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // PK, FK, Uniq
	Sources      []Column `json:"sources"`
	Destinations []Column `json:"destinations"`
}

type Table struct {
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	Constraints []Constraint `json:"constraints"`
	Schema      string       `json:"schema"`
	Indexes     []string     `json:"indexes"`
}

type MaterializedView struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
	Schema  string   `json:"schema"`
}

type View struct {
	Name            string `json:"name"`
	GenerationQuery string `json:"generate_query"`
}

type Schema struct {
	Name     string   `json:"name"`
	Tables   []string `json:"tables"`
	Views    []string `json:"views"`
	Database string   `json:"database"`
}
