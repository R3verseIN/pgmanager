package users

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"pgmanager/internal/auth"
	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ListUsers(pool *pgxpool.Pool, w http.ResponseWriter, r *http.Request) {
	rows, err := pool.Query(r.Context(), `
		SELECT u.username, MIN(u.access) AS access, MAX(u.created_at) AS created_at,
			ARRAY_AGG(u.database_name) AS databases,
			(SELECT allowed_ips FROM managed_users WHERE username = u.username LIMIT 1) AS allowed_ips
		FROM managed_users u
		INNER JOIN pg_catalog.pg_roles r ON r.rolname = u.username
		GROUP BY u.username
		ORDER BY MAX(u.created_at) DESC
	`)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	defer rows.Close()

	users := make([]UserRecord, 0)
	for rows.Next() {
		var u UserRecord
		var createdAt time.Time
		var allowedIpsRaw []byte
		if err := rows.Scan(&u.Username, &u.Access, &createdAt, &u.Databases, &allowedIpsRaw); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to scan row")
			return
		}
		u.CreatedAt = createdAt.Format(time.RFC3339)
		_ = json.Unmarshal(allowedIpsRaw, &u.AllowedIps)
		users = append(users, u)
	}

	core.WriteJSON(w, http.StatusOK, users)
}

func CreateUser(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Access = strings.TrimSpace(req.Access)

	validAccess := map[string]bool{"read": true, "write": true, "ddl": true, "full": true}
	if req.Username == "" {
		core.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Username) > 63 {
		core.WriteError(w, http.StatusBadRequest, "username too long (max 63 characters)")
		return
	}
	if !core.ValidName.MatchString(req.Username) {
		core.WriteError(w, http.StatusBadRequest, "invalid username: must start with letter or underscore, alphanumeric only")
		return
	}
	if len(req.Databases) == 0 {
		core.WriteError(w, http.StatusBadRequest, "at least one database is required")
		return
	}
	if !validAccess[req.Access] {
		core.WriteError(w, http.StatusBadRequest, "invalid access level (read, write, ddl, full)")
		return
	}

	for _, db := range req.Databases {
		if core.ProtectedDatabases[db] {
			core.WriteError(w, http.StatusBadRequest, "cannot grant access to system database: "+db)
			return
		}
	}

	password := req.Password
	if password != "" {
		if !core.ValidPassword(password) {
			core.WriteError(w, http.StatusBadRequest, "password must be 8-128 ASCII characters")
			return
		}
	} else {
		password = core.GeneratePassword(16)
	}

	var roleExists bool
	err := pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", req.Username).Scan(&roleExists)
	if err != nil || roleExists {
		core.WriteError(w, http.StatusBadRequest, "username already exists")
		return
	}

	ctx := r.Context()
	_, err = pool.Exec(ctx, "CREATE ROLE "+core.QuoteIdent(req.Username)+" WITH LOGIN PASSWORD "+core.QuoteLiteral(password))
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to create role: "+err.Error())
		return
	}

	for _, db := range req.Databases {
		var dbExists bool
		err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", db).Scan(&dbExists)
		if err != nil || !dbExists {
			RollbackUser(ctx, pool, baseDSN, req.Username)
			core.WriteError(w, http.StatusBadRequest, "database does not exist: "+db)
			return
		}

		if err := GrantAccess(ctx, pool, baseDSN, req.Username, db, req.Access); err != nil {
			RollbackUser(ctx, pool, baseDSN, req.Username)
			core.WriteError(w, http.StatusInternalServerError, "failed to grant access on "+db+": "+err.Error())
			return
		}

		if len(req.AllowedIps) == 0 {
			req.AllowedIps = []string{"0.0.0.0/0"}
		}
		ipsJSON, _ := json.Marshal(req.AllowedIps)
		_, err = pool.Exec(ctx,
			"INSERT INTO managed_users (username, database_name, access, allowed_ips) VALUES ($1, $2, $3, $4::jsonb)",
			req.Username, db, req.Access, string(ipsJSON),
		)
		if err != nil {
			RollbackUser(ctx, pool, baseDSN, req.Username)
			core.WriteError(w, http.StatusInternalServerError, "failed to save metadata: "+err.Error())
			return
		}
	}

	host := ResolveConnectionStringHost(r)
	connStr := "postgres://" + req.Username + ":" + password + "@" + host + "/" + req.Databases[0]
	if IsSSLEnabled(pool) {
		connStr += "?sslmode=require"
	}

	core.WriteJSON(w, http.StatusCreated, CreateUserResponse{
		Username:         req.Username,
		Password:         password,
		Databases:        req.Databases,
		ConnectionString: connStr,
		Access:           req.Access,
		AllowedIps:       req.AllowedIps,
		CreatedAt:        time.Now().Format(time.RFC3339),
	})

	user := auth.GetUserFromContext(r.Context())
	username := ""
	if user != nil {
		username = user.Username
	}
	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  username,
		Action:    "create_user",
		Detail:    map[string]interface{}{"target": req.Username, "databases": req.Databases, "access": req.Access, "allowedIps": req.AllowedIps},
		IPAddress: core.ClientIP(r),
	})
}

func UpdateUser(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	username := ExtractUserFromPath(r.URL.Path)
	if username == "" {
		core.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var roleExists bool
	err := pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", username).Scan(&roleExists)
	if err != nil || !roleExists {
		core.WriteError(w, http.StatusBadRequest, "user not found")
		return
	}

	ctx := r.Context()

	var newPassword string
	if req.GeneratePassword {
		newPassword = core.GeneratePassword(16)
		_, err = pool.Exec(ctx, "ALTER ROLE "+core.QuoteIdent(username)+" PASSWORD "+core.QuoteLiteral(newPassword))
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to update password: "+err.Error())
			return
		}
	} else if req.Password != "" {
		if !core.ValidPassword(req.Password) {
			core.WriteError(w, http.StatusBadRequest, "password must be 8-128 ASCII characters")
			return
		}
		_, err = pool.Exec(ctx, "ALTER ROLE "+core.QuoteIdent(username)+" PASSWORD "+core.QuoteLiteral(req.Password))
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to update password: "+err.Error())
			return
		}
		newPassword = req.Password
	}

	var currentAccess string
	var currentIps string
	_ = pool.QueryRow(ctx, "SELECT access, allowed_ips::text FROM managed_users WHERE username = $1 LIMIT 1", username).Scan(&currentAccess, &currentIps)
	if currentAccess == "" {
		currentAccess = "write"
	}
	if currentIps == "" {
		currentIps = "[\"0.0.0.0/0\"]"
	}
	if req.Access != "" {
		currentAccess = req.Access
	}
	if len(req.AllowedIps) > 0 {
		ipsBytes, _ := json.Marshal(req.AllowedIps)
		currentIps = string(ipsBytes)
	}

	if req.Databases != nil {
		existingDBs := GetUserDatabases(ctx, pool, username)
		existingMap := make(map[string]bool)
		for _, db := range existingDBs {
			existingMap[db] = true
		}
		requestedMap := make(map[string]bool)
		for _, db := range req.Databases {
			requestedMap[db] = true
		}

		for db := range existingMap {
			if !requestedMap[db] {
				if err := RevokeAccess(ctx, pool, baseDSN, username, db); err != nil {
					core.WriteError(w, http.StatusInternalServerError, "failed to revoke access: "+err.Error())
					return
				}
				_, _ = pool.Exec(ctx, "DELETE FROM managed_users WHERE username = $1 AND database_name = $2", username, db)
			}
		}

		for db := range requestedMap {
			if !existingMap[db] {
				if err := GrantAccess(ctx, pool, baseDSN, username, db, currentAccess); err != nil {
					core.WriteError(w, http.StatusInternalServerError, "failed to grant access: "+err.Error())
					return
				}
				_, _ = pool.Exec(ctx, "INSERT INTO managed_users (username, database_name, access, allowed_ips) VALUES ($1, $2, $3, $4::jsonb)", username, db, currentAccess, currentIps)
			}
		}
	}

	if req.Access != "" {
		validAccess := map[string]bool{"read": true, "write": true, "ddl": true, "full": true}
		if !validAccess[req.Access] {
			core.WriteError(w, http.StatusBadRequest, "invalid access level (read, write, ddl, full)")
			return
		}

		databases := GetUserDatabases(ctx, pool, username)
		for _, db := range databases {
			if err := RevokeAccess(ctx, pool, baseDSN, username, db); err != nil {
				core.WriteError(w, http.StatusInternalServerError, "failed to revoke access: "+err.Error())
				return
			}
			if err := GrantAccess(ctx, pool, baseDSN, username, db, req.Access); err != nil {
				core.WriteError(w, http.StatusInternalServerError, "failed to grant access: "+err.Error())
				return
			}
		}

		_, err = pool.Exec(ctx, "UPDATE managed_users SET access = $1 WHERE username = $2", req.Access, username)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to update metadata: "+err.Error())
			return
		}
	}
	if len(req.AllowedIps) > 0 {
		ipsJSON, _ := json.Marshal(req.AllowedIps)
		_, err = pool.Exec(ctx, "UPDATE managed_users SET allowed_ips = $1::jsonb WHERE username = $2", string(ipsJSON), username)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "failed to update allowed_ips: "+err.Error())
			return
		}
	}

	resp := map[string]string{"status": "updated"}
	if newPassword != "" {
		resp["password"] = newPassword
	}
	core.WriteJSON(w, http.StatusOK, resp)

	user := auth.GetUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Username
	}
	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  actor,
		Action:    "update_user",
		Detail:    map[string]interface{}{"target": username, "access": req.Access, "allowedIps": req.AllowedIps},
		IPAddress: core.ClientIP(r),
	})
}

func AddUserDatabase(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	username := ExtractUserFromPath(r.URL.Path)
	if username == "" {
		core.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}

	var req AddDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Database = strings.TrimSpace(req.Database)

	if req.Database == "" {
		core.WriteError(w, http.StatusBadRequest, "database is required")
		return
	}
	if core.ProtectedDatabases[req.Database] {
		core.WriteError(w, http.StatusBadRequest, "cannot grant access to system database")
		return
	}

	ctx := r.Context()

	var roleExists bool
	err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", username).Scan(&roleExists)
	if err != nil || !roleExists {
		core.WriteError(w, http.StatusBadRequest, "user not found")
		return
	}

	var dbExists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", req.Database).Scan(&dbExists)
	if err != nil || !dbExists {
		core.WriteError(w, http.StatusBadRequest, "database does not exist")
		return
	}

	var alreadyHas bool
	err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM managed_users WHERE username = $1 AND database_name = $2)", username, req.Database).Scan(&alreadyHas)
	if err != nil || alreadyHas {
		core.WriteError(w, http.StatusBadRequest, "user already has access to this database")
		return
	}

	var access string
	err = pool.QueryRow(ctx, "SELECT access FROM managed_users WHERE username = $1 LIMIT 1", username).Scan(&access)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to get user access level")
		return
	}

	if err := GrantAccess(ctx, pool, baseDSN, username, req.Database, access); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to grant access: "+err.Error())
		return
	}

	_, err = pool.Exec(ctx,
		"INSERT INTO managed_users (username, database_name, access) VALUES ($1, $2, $3)",
		username, req.Database, access,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to save metadata: "+err.Error())
		return
	}

	core.WriteJSON(w, http.StatusCreated, map[string]string{"status": "granted"})

	user := auth.GetUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Username
	}
	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  actor,
		Action:    "add_user_database",
		Database:  req.Database,
		Detail:    map[string]interface{}{"target": username},
		IPAddress: core.ClientIP(r),
	})
}

func RemoveUserDatabase(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	username, db := ExtractUserDBFromPath(r.URL.Path)
	if username == "" || db == "" {
		core.WriteError(w, http.StatusBadRequest, "username and database are required")
		return
	}

	ctx := r.Context()

	_, err := pool.Exec(ctx, "DELETE FROM managed_users WHERE username = $1 AND database_name = $2", username, db)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "failed to remove metadata")
		return
	}

	_ = RevokeAccess(ctx, pool, baseDSN, username, db)
	remaining := GetUserDatabases(ctx, pool, username)
	if len(remaining) == 0 {
		_, _ = pool.Exec(ctx, "DROP OWNED BY "+core.QuoteIdent(username)+" CASCADE")
		_, _ = pool.Exec(ctx, "DROP ROLE IF EXISTS "+core.QuoteIdent(username))
	}

	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "removed"})

	user := auth.GetUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Username
	}
	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  actor,
		Action:    "remove_user_database",
		Database:  db,
		Detail:    map[string]interface{}{"target": username},
		IPAddress: core.ClientIP(r),
	})
}

func DeleteUser(pool *pgxpool.Pool, baseDSN string, w http.ResponseWriter, r *http.Request) {
	username := ExtractUserFromPath(r.URL.Path)
	if username == "" {
		core.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}

	ctx := r.Context()

	databases := GetUserDatabases(ctx, pool, username)
	for _, db := range databases {
		WithDatabase(ctx, baseDSN, db, func(conn *pgx.Conn) error {
			conn.Exec(ctx, "DROP OWNED BY "+core.QuoteIdent(username)+" CASCADE")
			return nil
		})
	}

	_, _ = pool.Exec(ctx, "DROP ROLE IF EXISTS "+core.QuoteIdent(username))
	_, _ = pool.Exec(ctx, "DELETE FROM managed_users WHERE username = $1", username)

	core.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	user := auth.GetUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Username
	}
	core.WriteAuditLog(pool, r.Context(), core.AuditEntry{
		Username:  actor,
		Action:    "delete_user",
		Detail:    map[string]interface{}{"target": username},
		IPAddress: core.ClientIP(r),
	})
}
