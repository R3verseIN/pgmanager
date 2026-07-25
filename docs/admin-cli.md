# pgmanager Admin CLI

Interactive recovery tool for webapp user management. Runs inside the DB container with direct PostgreSQL access.

## Quick Start

```sh
docker compose exec -T db python3 /scripts/admin.py
```

## Commands

```
=========================================
  pgmanager Admin CLI
=========================================

1. List users
2. Create user
3. Delete user
4. Reset password
5. Change role
6. Quit
```

### 1. List Users

Shows all webapp users with their roles and creation dates.

```
Choose: 1

Username             Role       Created
--------------------------------------------------
admin                admin      2026-07-25 10:50
```

### 2. Create User

Creates a new webapp login account.

```
Choose: 2

Username: alice
Password (leave blank to generate): mypass123
Role (admin/dev/viewer): dev

User 'alice' created with role 'dev'.
```

**Roles:**
- `admin` — Full access. Can manage users, databases, and settings.
- `dev` — Can query data. Assigned specific databases.
- `viewer` — Read-only access.

**Password:** Leave blank to auto-generate a secure 24-character password.

### 3. Delete User

Removes a webapp user and invalidates their sessions.

```
Choose: 3

Username             Role
------------------------------
admin                admin
alice                dev

Username to delete (or 'q' to cancel): alice
Delete 'alice'? This cannot be undone. (yes/no): yes

User 'alice' deleted.
```

**Protection:** Cannot delete the last admin user.

### 4. Reset Password

Changes a user's password and invalidates all their sessions.

```
Choose: 4

Username to reset (or 'q' to cancel): alice
New password (leave blank to generate): newpass123

Password reset for 'alice'. All sessions invalidated.
```

**Password:** Leave blank to auto-generate a secure 24-character password.

### 5. Change Role

Changes a user's role. Invalidates their sessions.

```
Choose: 5

Username to update (or 'q' to cancel): alice
Current role: dev
New role (admin/dev/viewer): viewer

'alice' role changed from 'dev' to 'viewer'. Sessions invalidated.
```

**Protection:** Cannot demote the last admin user.

## Recovery Scenarios

### Scenario 1: Locked Out (auth_users dropped)

If the `auth_users` table is dropped while the setup wizard has already been completed, the webapp login page shows but you cannot log in. The setup wizard will not reappear.

**Recovery:**

```sh
# Step 1: Restart the app to recreate tables
docker compose restart app

# Step 2: Create a new admin via CLI
docker compose exec -T db python3 /scripts/admin.py
# Choose: 2 (Create user) → enter username, password, admin role

# Step 3: Login to webapp
```

### Scenario 2: Password Forgotten

If you forgot your admin password and cannot log in.

**Recovery:**

```sh
# Reset via CLI
docker compose exec -T db python3 /scripts/admin.py
# Choose: 4 (Reset password) → enter username, new password

# Or generate a random password
docker compose exec -T db python3 /scripts/admin.py
# Choose: 4 → enter username → leave password blank
```

### Scenario 3: Last Admin Demoted

If the only admin user was accidentally changed to a lower role.

**Recovery:**

```sh
# Promote back to admin via CLI
docker compose exec -T db python3 /scripts/admin.py
# Choose: 5 (Change role) → enter username → admin
```

### Scenario 4: Complete Data Wipe

If both `auth_users` and `system_config` tables are dropped (or the entire database is recreated).

**Recovery:**

The webapp setup wizard will appear automatically on next visit. Create an admin account through the web UI.

### Scenario 5: Webapp Down, Need Direct Access

If the Go app is crashed or unreachable, you can still manage users via the CLI (it connects directly to PostgreSQL).

```sh
# List users
docker compose exec -T db python3 /scripts/admin.py
# Choose: 1

# Create user
docker compose exec -T db python3 /scripts/admin.py
# Choose: 2
```

## Security Model

### Setup Wizard is One-Time-Only

| Scenario | Setup Wizard Shows? |
|---|---|
| Fresh install (no data) | Yes |
| After admin created | No |
| auth_users dropped | No (locked out — use CLI) |
| Both tables dropped | Yes (full wipe) |
| setup flag + auth_users dropped | No (locked out — use CLI) |

### Why This Design?

- **Prevents unauthorized re-setup:** An attacker who drops `auth_users` cannot create a new admin via the web UI.
- **Recovery requires container access:** You must have `docker compose exec` access to the DB container.
- **Audit trail:** All user management goes through explicit actions, not automatic wizards.

## Password Policy

| Rule | Value |
|---|---|
| Minimum length | 8 characters |
| Maximum length | 72 characters (bcrypt limit) |
| Allowed characters | Alphanumeric, underscore, hyphen |
| Hash algorithm | bcrypt (via pgcrypto) |

## Environment Variables

| Variable | Description | Required |
|---|---|---|
| `POSTGRES_PASSWORD` | PostgreSQL superuser password | Yes |
| `PGBOUNCER_AUTH_PASSWORD` | PgBouncer auth proxy password | Yes |

These are set in your `.env` file and used by the init script on every startup.
