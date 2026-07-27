package sshcmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Exec runs a command on a remote host via SSH.
func Exec(keyPath, user, host, command string) (string, string, error) {
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-i", keyPath,
		fmt.Sprintf("%s@%s", user, host),
		command,
	}

	cmd := exec.Command("ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// ExecPSQL runs a psql command on the remote PostgreSQL host via SSH.
func ExecPSQL(keyPath, user, host, dbname, sql string) (string, error) {
	escaped := strings.ReplaceAll(sql, "'", "'\\''")
	cmd := fmt.Sprintf("psql -v ON_ERROR_STOP=1 -U pgmanager -d %s -c '%s'", dbname, escaped)
	stdout, stderr, err := Exec(keyPath, user, host, cmd)
	if err != nil {
		return "", fmt.Errorf("psql via SSH: %w: %s", err, stderr)
	}
	return stdout, nil
}

// ReloadPostgreSQL sends pg_ctl reload to the remote PostgreSQL via SSH.
func ReloadPostgreSQL(keyPath, user, host string) error {
	_, stderr, err := Exec(keyPath, user, host, "su-exec postgres pg_ctl reload -D /var/lib/postgresql/data")
	if err != nil {
		return fmt.Errorf("pg_ctl reload via SSH: %w: %s", err, stderr)
	}
	return nil
}

// RestartPostgreSQL signals the watcher to restart PostgreSQL via SSH.
// The watcher runs as a background process in the db container and handles
// the actual pg_ctl restart, avoiding PID 1 issues in Docker.
func RestartPostgreSQL(keyPath, user, host string) error {
	_, stderr, err := Exec(keyPath, user, host,
		"touch /var/lib/postgresql/data/pgmanager-restart-signal")
	if err != nil {
		return fmt.Errorf("restart signal via SSH: %w: %s", err, stderr)
	}
	return nil
}
