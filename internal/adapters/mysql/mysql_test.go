package mysql

import "testing"

func TestParseMySQLDBName(t *testing.T) {
	tests := []struct {
		dsn  string
		want string
	}{
		{"root:pass@tcp(localhost:3306)/mydb", "mydb"},
		{"root:pass@tcp(localhost:3306)/mydb?charset=utf8", "mydb"},
		{"user:password@tcp(10.0.0.1:3306)/production", "production"},
		{"root:pass@tcp(localhost:3306)/", "mysql"},
		{"invalid", "mysql"},
		{"", "mysql"},
	}
	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			if got := parseMySQLDBName(tt.dsn); got != tt.want {
				t.Errorf("parseMySQLDBName(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}
