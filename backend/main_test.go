package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "password")

	if err := os.WriteFile(path, []byte("  testpass123  \n"), 0600); err != nil {
		t.Fatal(err)
	}

	got := readPassword(path)
	if got != "testpass123" {
		t.Fatalf("expected 'testpass123', got %q", got)
	}
}

func TestReadPassword_Whitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "password")

	if err := os.WriteFile(path, []byte("\n\n  hello  \n\n"), 0600); err != nil {
		t.Fatal(err)
	}

	got := readPassword(path)
	if got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}
}

func TestBuildDatabaseURL_FromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/dbname?sslmode=disable")

	got := buildDatabaseURL()
	if got != "postgres://user:pass@host:5432/dbname?sslmode=disable" {
		t.Fatalf("unexpected URL: %s", got)
	}
}

func TestBuildDatabaseURL_FromSecretPath(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PGHOST", "testhost")
	t.Setenv("PGPORT", "5433")
	t.Setenv("PGUSER", "testuser")
	t.Setenv("PGDATABASE", "testdb")
	t.Setenv("PGSSLMODE", "require")

	dir := t.TempDir()
	path := filepath.Join(dir, "password")
	os.WriteFile(path, []byte("mypassword"), 0600)
	t.Setenv("SECRET_PATH", path)

	got := buildDatabaseURL()
	expected := "postgres://testuser:mypassword@testhost:5433/testdb?sslmode=require"
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestBuildDatabaseURL_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PGHOST", "")
	t.Setenv("PGPORT", "")
	t.Setenv("PGUSER", "")
	t.Setenv("PGDATABASE", "")
	t.Setenv("PGSSLMODE", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "password")
	os.WriteFile(path, []byte("secret"), 0600)
	t.Setenv("SECRET_PATH", path)

	got := buildDatabaseURL()
	expected := "postgres://pgmanager:secret@localhost:5432/pgmanager?sslmode=disable"
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}
