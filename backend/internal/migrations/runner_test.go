package migrations

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMigrationFilesAreDeterministicAndIgnoreDownFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"000029_b.up.sql", "000029_b.down.sql", "schema.sql", "000028_a.up.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := migrationFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"000028_a.up.sql", "000029_b.up.sql"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migrationFiles() = %v, want %v", got, want)
	}
}

func TestChecksumIsStableAndContentSensitive(t *testing.T) {
	a := checksum([]byte("SELECT 1"))
	if a != checksum([]byte("SELECT 1")) {
		t.Fatal("same content produced different checksums")
	}
	if a == checksum([]byte("SELECT 2")) {
		t.Fatal("different content produced the same checksum")
	}
}
