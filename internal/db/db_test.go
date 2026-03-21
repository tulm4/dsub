package db

import (
	"fmt"
	"testing"
	"testing/fstest"
	"time"
)

func TestNewSelect(t *testing.T) {
	tests := []struct {
		name    string
		builder *QueryBuilder
		want    string
	}{
		{
			name:    "basic select all",
			builder: NewSelect("subscribers"),
			want:    "SELECT * FROM subscribers",
		},
		{
			name:    "select specific columns",
			builder: NewSelect("subscribers", "supi", "gpsi", "auth_method"),
			want:    "SELECT supi, gpsi, auth_method FROM subscribers",
		},
		{
			name:    "select with where",
			builder: NewSelect("subscribers", "supi", "gpsi").Where("supi = $1"),
			want:    "SELECT supi, gpsi FROM subscribers WHERE supi = $1",
		},
		{
			name: "select with multiple where clauses",
			builder: NewSelect("subscribers", "supi").
				Where("supi = $1").
				Where("auth_method = $2"),
			want: "SELECT supi FROM subscribers WHERE supi = $1 AND auth_method = $2",
		},
		{
			name: "select with order by",
			builder: NewSelect("subscribers", "supi").
				Where("supi = $1").
				OrderBy("created_at DESC"),
			want: "SELECT supi FROM subscribers WHERE supi = $1 ORDER BY created_at DESC",
		},
		{
			name: "select with limit",
			builder: NewSelect("subscribers", "supi").
				Limit(10),
			want: "SELECT supi FROM subscribers LIMIT 10",
		},
		{
			name: "select with offset",
			builder: NewSelect("subscribers", "supi").
				Limit(10).
				Offset(20),
			want: "SELECT supi FROM subscribers LIMIT 10 OFFSET 20",
		},
		{
			name: "complex select",
			builder: NewSelect("authentication_data", "supi", "auth_method", "sqn").
				Where("supi = $1").
				Where("auth_method = $2").
				OrderBy("created_at DESC").
				Limit(5).
				Offset(10),
			want: "SELECT supi, auth_method, sqn FROM authentication_data WHERE supi = $1 AND auth_method = $2 ORDER BY created_at DESC LIMIT 5 OFFSET 10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.builder.Build()
			if got != tt.want {
				t.Errorf("Build() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestNewInsert(t *testing.T) {
	tests := []struct {
		name    string
		builder *QueryBuilder
		want    string
	}{
		{
			name:    "basic insert",
			builder: NewInsert("subscribers", "supi", "gpsi"),
			want:    "INSERT INTO subscribers (supi, gpsi) VALUES ($1, $2)",
		},
		{
			name:    "insert with three columns",
			builder: NewInsert("authentication_data", "supi", "auth_method", "sqn"),
			want:    "INSERT INTO authentication_data (supi, auth_method, sqn) VALUES ($1, $2, $3)",
		},
		{
			name: "insert with on conflict",
			builder: NewInsert("subscribers", "supi", "gpsi").
				OnConflict("(supi) DO UPDATE SET gpsi = EXCLUDED.gpsi"),
			want: "INSERT INTO subscribers (supi, gpsi) VALUES ($1, $2) ON CONFLICT (supi) DO UPDATE SET gpsi = EXCLUDED.gpsi",
		},
		{
			name: "insert with on conflict do nothing",
			builder: NewInsert("subscribers", "supi").
				OnConflict("(supi) DO NOTHING"),
			want: "INSERT INTO subscribers (supi) VALUES ($1) ON CONFLICT (supi) DO NOTHING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.builder.Build()
			if got != tt.want {
				t.Errorf("Build() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestNewUpdate(t *testing.T) {
	tests := []struct {
		name    string
		builder *QueryBuilder
		want    string
	}{
		{
			name: "basic update",
			builder: NewUpdate("subscribers").
				Set("gpsi = $1").
				Where("supi = $2"),
			want: "UPDATE subscribers SET gpsi = $1 WHERE supi = $2",
		},
		{
			name: "update multiple columns",
			builder: NewUpdate("authentication_data").
				Set("auth_method = $1").
				Set("sqn = $2").
				Where("supi = $3"),
			want: "UPDATE authentication_data SET auth_method = $1, sqn = $2 WHERE supi = $3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.builder.Build()
			if got != tt.want {
				t.Errorf("Build() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestNewDelete(t *testing.T) {
	tests := []struct {
		name    string
		builder *QueryBuilder
		want    string
	}{
		{
			name:    "delete with where",
			builder: NewDelete("subscribers").Where("supi = $1"),
			want:    "DELETE FROM subscribers WHERE supi = $1",
		},
		{
			name: "delete with multiple conditions",
			builder: NewDelete("amf_registrations").
				Where("supi = $1").
				Where("access_type = $2"),
			want: "DELETE FROM amf_registrations WHERE supi = $1 AND access_type = $2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.builder.Build()
			if got != tt.want {
				t.Errorf("Build() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestParseMigrations(t *testing.T) {
	t.Run("valid migrations", func(t *testing.T) {
		fs := fstest.MapFS{
			"001_create_subscribers.up.sql": &fstest.MapFile{
				Data: []byte("CREATE TABLE subscribers (supi TEXT PRIMARY KEY);"),
			},
			"002_create_auth_data.up.sql": &fstest.MapFile{
				Data: []byte("CREATE TABLE authentication_data (supi TEXT PRIMARY KEY);"),
			},
			"001_create_subscribers.down.sql": &fstest.MapFile{
				Data: []byte("DROP TABLE subscribers;"),
			},
		}

		migrations, err := ParseMigrations(fs)
		if err != nil {
			t.Fatalf("ParseMigrations() error = %v", err)
		}

		if len(migrations) != 2 {
			t.Fatalf("expected 2 migrations, got %d", len(migrations))
		}

		if migrations[0].Version != 1 {
			t.Errorf("first migration version = %d, want 1", migrations[0].Version)
		}
		if migrations[0].Description != "create_subscribers" {
			t.Errorf("first migration description = %q, want %q", migrations[0].Description, "create_subscribers")
		}
		if migrations[0].SQL != "CREATE TABLE subscribers (supi TEXT PRIMARY KEY);" {
			t.Errorf("first migration SQL = %q", migrations[0].SQL)
		}

		if migrations[1].Version != 2 {
			t.Errorf("second migration version = %d, want 2", migrations[1].Version)
		}
		if migrations[1].Description != "create_auth_data" {
			t.Errorf("second migration description = %q, want %q", migrations[1].Description, "create_auth_data")
		}
	})

	t.Run("sorts by version", func(t *testing.T) {
		fs := fstest.MapFS{
			"003_third.up.sql": &fstest.MapFile{
				Data: []byte("SELECT 3;"),
			},
			"001_first.up.sql": &fstest.MapFile{
				Data: []byte("SELECT 1;"),
			},
			"002_second.up.sql": &fstest.MapFile{
				Data: []byte("SELECT 2;"),
			},
		}

		migrations, err := ParseMigrations(fs)
		if err != nil {
			t.Fatalf("ParseMigrations() error = %v", err)
		}

		if len(migrations) != 3 {
			t.Fatalf("expected 3 migrations, got %d", len(migrations))
		}

		for i, want := range []int{1, 2, 3} {
			if migrations[i].Version != want {
				t.Errorf("migration[%d].Version = %d, want %d", i, migrations[i].Version, want)
			}
		}
	})

	t.Run("invalid filename no underscore", func(t *testing.T) {
		fs := fstest.MapFS{
			"badname.up.sql": &fstest.MapFile{
				Data: []byte("SELECT 1;"),
			},
		}

		_, err := ParseMigrations(fs)
		if err == nil {
			t.Fatal("expected error for invalid filename, got nil")
		}
	})

	t.Run("invalid version number", func(t *testing.T) {
		fs := fstest.MapFS{
			"abc_description.up.sql": &fstest.MapFile{
				Data: []byte("SELECT 1;"),
			},
		}

		_, err := ParseMigrations(fs)
		if err == nil {
			t.Fatal("expected error for non-numeric version, got nil")
		}
	})

	t.Run("empty filesystem", func(t *testing.T) {
		fs := fstest.MapFS{}

		migrations, err := ParseMigrations(fs)
		if err != nil {
			t.Fatalf("ParseMigrations() error = %v", err)
		}
		if len(migrations) != 0 {
			t.Errorf("expected 0 migrations, got %d", len(migrations))
		}
	})
}

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"MaxConns", cfg.MaxConns, int32(50)},
		{"MinConns", cfg.MinConns, int32(10)},
		{"MaxConnLifetime", cfg.MaxConnLifetime, 5 * time.Minute},
		{"MaxConnIdleTime", cfg.MaxConnIdleTime, 5 * time.Minute},
		{"HealthCheckPeriod", cfg.HealthCheckPeriod, 30 * time.Second},
		{"DSN empty", cfg.DSN, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestDefaultTxOptions(t *testing.T) {
	opts := DefaultTxOptions()

	if opts.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", opts.MaxRetries)
	}
	if opts.MaxRetryDelay != 1*time.Second {
		t.Errorf("MaxRetryDelay = %v, want 1s", opts.MaxRetryDelay)
	}
	if opts.ReadOnly != false {
		t.Errorf("ReadOnly = %v, want false", opts.ReadOnly)
	}
}

func TestIsSerializationConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic error",
			err:  fmt.Errorf("some error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSerializationConflict(tt.err); got != tt.want {
				t.Errorf("isSerializationConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}
