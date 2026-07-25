#!/usr/bin/env python3
"""
pgmanager Admin CLI — Interactive recovery tool for webapp user management.

Usage:
    docker compose exec -T db python3 /scripts/admin.py
"""

import os
import re
import secrets
import subprocess
import sys
from pathlib import Path

PASSWORD_FILE = Path("/var/lib/postgresql/data/pgmanager-password")

BANNER = """
=========================================
  pgmanager Admin CLI
=========================================
"""

MENU = """
1. List users
2. Create user
3. Delete user
4. Reset password
5. Change role
6. Quit
"""


def fatal(msg: str) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(1)


def generate_password(length: int = 24) -> str:
    alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
    return "".join(secrets.choice(alphabet) for _ in range(length))


def validate_password(password: str) -> None:
    if len(password) < 8:
        fatal("Password must be at least 8 characters")
    if len(password) > 72:
        fatal("Password must be at most 72 characters")
    if not re.match(r"^[a-zA-Z0-9_-]+$", password):
        fatal("Password contains invalid characters (only alphanumeric, _, - allowed)")


def validate_username(username: str) -> None:
    if not re.match(r"^[a-zA-Z0-9_-]+$", username):
        fatal("Username contains invalid characters (only alphanumeric, _, - allowed)")


def run_sql(sql: str) -> str:
    password = PASSWORD_FILE.read_text().strip()
    result = subprocess.run(
        ["psql", "-v", "ON_ERROR_STOP=1", "-U", "pgmanager", "-d", "pgmanager", "-t", "-A", "-c", sql],
        capture_output=True,
        env={**os.environ, "PGPASSWORD": password},
    )
    if result.returncode != 0:
        fatal(f"psql failed: {result.stderr.decode().strip()}")
    return result.stdout.decode().strip()


def hash_password(password: str) -> str:
    run_sql("CREATE EXTENSION IF NOT EXISTS pgcrypto;")
    return run_sql(f"SELECT crypt('{password}', gen_salt('bf'));")


def prompt(msg: str) -> str:
    print(msg, end="", flush=True)
    return input().strip()


def list_users() -> None:
    output = run_sql("SELECT username || ' | ' || role || ' | ' || TO_CHAR(created_at, 'YYYY-MM-DD HH24:MI') FROM auth_users ORDER BY id;")
    if not output:
        print("\nNo users found.")
        return

    print(f"\n{'Username':<20} {'Role':<10} {'Created':<20}")
    print("-" * 50)
    for line in output.strip().split("\n"):
        parts = line.split(" | ")
        if len(parts) == 3:
            print(f"{parts[0]:<20} {parts[1]:<10} {parts[2]:<20}")


def create_user() -> None:
    username = prompt("\nUsername: ")
    if not username:
        print("Aborted.")
        return
    validate_username(username)

    existing = run_sql(f"SELECT COUNT(*) FROM auth_users WHERE username = '{username}';")
    if existing != "0":
        print(f"ERROR: User '{username}' already exists.")
        return

    password = prompt("Password (leave blank to generate): ")
    if not password:
        password = generate_password()
        print(f"Generated: {password}")
    validate_password(password)

    print("Role (admin/dev/viewer): ", end="", flush=True)
    role = input().strip()
    if role not in ("admin", "dev", "viewer"):
        print("ERROR: Role must be admin, dev, or viewer.")
        return

    password_hash = hash_password(password)
    run_sql(f"INSERT INTO auth_users (username, password_hash, role) VALUES ('{username}', '{password_hash}', '{role}');")

    print(f"\nUser '{username}' created with role '{role}'.")


def delete_user() -> None:
    users = run_sql("SELECT username || ' (' || role || ')' FROM auth_users ORDER BY id;")
    if not users:
        print("\nNo users found.")
        return

    print(f"\n{'Username':<20} {'Role':<10}")
    print("-" * 30)
    for line in users.strip().split("\n"):
        parts = line.rsplit(" (", 1)
        if len(parts) == 2:
            role = parts[1].rstrip(")")
            print(f"{parts[0]:<20} {role:<10}")

    username = prompt("\nUsername to delete (or 'q' to cancel): ")
    if username in ("q", ""):
        print("Aborted.")
        return
    validate_username(username)

    admin_count = run_sql("SELECT COUNT(*) FROM auth_users WHERE role = 'admin';")
    user_role = run_sql(f"SELECT role FROM auth_users WHERE username = '{username}';")

    if not user_role:
        print(f"ERROR: User '{username}' not found.")
        return

    if user_role == "admin" and int(admin_count) <= 1:
        print("ERROR: Cannot delete the last admin user.")
        return

    confirm = prompt(f"Delete '{username}'? This cannot be undone. (yes/no): ")
    if confirm.lower() != "yes":
        print("Aborted.")
        return

    run_sql(f"DELETE FROM sessions WHERE user_id = (SELECT id FROM auth_users WHERE username = '{username}');")
    run_sql(f"DELETE FROM auth_users WHERE username = '{username}';")

    print(f"\nUser '{username}' deleted.")


def reset_password() -> None:
    users = run_sql("SELECT username || ' (' || role || ')' FROM auth_users ORDER BY id;")
    if not users:
        print("\nNo users found.")
        return

    print(f"\n{'Username':<20} {'Role':<10}")
    print("-" * 30)
    for line in users.strip().split("\n"):
        parts = line.rsplit(" (", 1)
        if len(parts) == 2:
            role = parts[1].rstrip(")")
            print(f"{parts[0]:<20} {role:<10}")

    username = prompt("\nUsername to reset (or 'q' to cancel): ")
    if username in ("q", ""):
        print("Aborted.")
        return
    validate_username(username)

    if run_sql(f"SELECT COUNT(*) FROM auth_users WHERE username = '{username}';") == "0":
        print(f"ERROR: User '{username}' not found.")
        return

    password = prompt("New password (leave blank to generate): ")
    if not password:
        password = generate_password()
        print(f"Generated: {password}")
    validate_password(password)

    password_hash = hash_password(password)
    run_sql(f"UPDATE auth_users SET password_hash = '{password_hash}', updated_at = NOW() WHERE username = '{username}';")
    run_sql(f"DELETE FROM sessions WHERE user_id = (SELECT id FROM auth_users WHERE username = '{username}');")

    print(f"\nPassword reset for '{username}'. All sessions invalidated.")


def change_role() -> None:
    users = run_sql("SELECT username || ' (' || role || ')' FROM auth_users ORDER BY id;")
    if not users:
        print("\nNo users found.")
        return

    print(f"\n{'Username':<20} {'Role':<10}")
    print("-" * 30)
    for line in users.strip().split("\n"):
        parts = line.rsplit(" (", 1)
        if len(parts) == 2:
            role = parts[1].rstrip(")")
            print(f"{parts[0]:<20} {role:<10}")

    username = prompt("\nUsername to update (or 'q' to cancel): ")
    if username in ("q", ""):
        print("Aborted.")
        return
    validate_username(username)

    current_role = run_sql(f"SELECT role FROM auth_users WHERE username = '{username}';")
    if not current_role:
        print(f"ERROR: User '{username}' not found.")
        return

    print(f"Current role: {current_role}")
    print("New role (admin/dev/viewer): ", end="", flush=True)
    new_role = input().strip()
    if new_role not in ("admin", "dev", "viewer"):
        print("ERROR: Role must be admin, dev, or viewer.")
        return

    if new_role == current_role:
        print("No change.")
        return

    # Prevent demoting the last admin
    if current_role == "admin" and new_role != "admin":
        admin_count = run_sql("SELECT COUNT(*) FROM auth_users WHERE role = 'admin';")
        if int(admin_count) <= 1:
            print("ERROR: Cannot demote the last admin user.")
            return

    run_sql(f"UPDATE auth_users SET role = '{new_role}', updated_at = NOW() WHERE username = '{username}';")
    run_sql(f"DELETE FROM sessions WHERE user_id = (SELECT id FROM auth_users WHERE username = '{username}');")

    print(f"\n'{username}' role changed from '{current_role}' to '{new_role}'. Sessions invalidated.")


def main() -> int:
    if not PASSWORD_FILE.exists():
        fatal(f"Password file not found: {PASSWORD_FILE}")

    print(BANNER)

    actions = {
        "1": list_users,
        "2": create_user,
        "3": delete_user,
        "4": reset_password,
        "5": change_role,
    }

    while True:
        print(MENU)
        choice = prompt("Choose: ")

        if choice == "6":
            print("Bye.")
            break

        action = actions.get(choice)
        if action:
            try:
                action()
            except KeyboardInterrupt:
                print("\nAborted.")
            except Exception as e:
                print(f"\nERROR: {e}", file=sys.stderr)
        else:
            print("Invalid choice.")

    return 0


if __name__ == "__main__":
    sys.exit(main())
