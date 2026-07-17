package gcp

import "testing"

func TestParseCloudSqlVersionToSemver(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{"Postgres 15", "POSTGRES_15", "15.0.0", false},
		{"Postgres 14", "POSTGRES_14", "14.0.0", false},
		{"MySQL 8.0", "MYSQL_8_0", "8.0.0", false},
		{"MySQL 5.7", "MYSQL_5_7", "5.7.0", false},
		{"SQL Server 2019 Standard", "SQLSERVER_2019_STANDARD", "2019.0.0", false},
		{"SQL Server 2017 Enterprise", "SQLSERVER_2017_ENTERPRISE", "2017.0.0", false},
		{"SQL Server 2022 Web", "SQLSERVER_2022_WEB", "2022.0.0", false},
		{"Empty String", "", "", true},
		{"Invalid Format", "ORACLE_12C", "", true},
		{"No Version", "POSTGRES", "", true},
		{"Just Number", "15", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCloudSqlVersionToSemver(tc.input)

			if (err != nil) != tc.wantErr {
				t.Errorf("ParseCloudSqlVersionToSemver() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !tc.wantErr && got != tc.expected {
				t.Errorf("ParseCloudSqlVersionToSemver() = %v, want %v", got, tc.expected)
			}
		})
	}
}
