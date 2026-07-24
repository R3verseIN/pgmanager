import { z } from "zod";

export const DatabaseSchema = z.object({
  name: z.string(),
  protected: z.boolean(),
});

export type Database = z.infer<typeof DatabaseSchema>;

export const DatabasesResponseSchema = z.array(DatabaseSchema);

export const CreateDatabaseSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "name is required")
    .max(63, "name too long (max 63 characters)")
    .regex(/^[a-zA-Z_][a-zA-Z0-9_]*$/, "must start with letter or underscore, alphanumeric only"),
});

export const ErrorResponseSchema = z.object({
  error: z.string(),
});

export const UserSchema = z.object({
  username: z.string(),
  databases: z.array(z.string()),
  access: z.enum(["read", "write", "ddl", "full"]),
  allowedIps: z.array(z.string()).default(["0.0.0.0/0"]),
  createdAt: z.string(),
});

export type User = z.infer<typeof UserSchema>;

export const UsersResponseSchema = z.array(UserSchema);

export const CreateUserRequestSchema = z.object({
  databases: z.array(z.string()).min(1, "at least one database is required"),
  username: z
    .string()
    .trim()
    .min(1, "username is required")
    .max(63, "username too long (max 63 characters)")
    .regex(/^[a-zA-Z_][a-zA-Z0-9_]*$/, "must start with letter or underscore, alphanumeric only"),
  access: z.enum(["read", "write", "ddl", "full"]),
  password: z.string().optional(),
});

export const CreateUserResponseSchema = z.object({
  username: z.string(),
  password: z.string(),
  databases: z.array(z.string()),
  connectionString: z.string(),
  access: z.enum(["read", "write", "ddl", "full"]),
  allowedIps: z.array(z.string()).default(["0.0.0.0/0"]),
  createdAt: z.string(),
});

export const UpdateUserRequestSchema = z.object({
  password: z.string().optional(),
  access: z.enum(["read", "write", "ddl", "full"]).optional(),
  generatePassword: z.boolean().optional(),
  allowedIps: z.array(z.string()).optional(),
});

export const AddDatabaseRequestSchema = z.object({
  database: z.string().min(1, "database is required"),
});

export const LoginRequestSchema = z.object({
  username: z.string().min(1, "username is required"),
  password: z.string().min(1, "password is required"),
});

export const SetupRequestSchema = z.object({
  username: z.string().min(3, "username must be at least 3 characters"),
  password: z.string().min(8, "password must be at least 8 characters"),
});

export const AuthUserSchema = z.object({
  username: z.string(),
  role: z.enum(["admin", "dev", "viewer"]),
});

export type AuthUser = z.infer<typeof AuthUserSchema>;

export const AuthUsersResponseSchema = z.array(AuthUserSchema);

export const AuthUserListItemSchema = z.object({
  id: z.number(),
  username: z.string(),
  role: z.enum(["admin", "dev", "viewer"]),
  databases: z.array(z.string()).optional(),
  createdAt: z.string(),
});

export type AuthUserListItem = z.infer<typeof AuthUserListItemSchema>;

export const AuthUserListResponseSchema = z.array(AuthUserListItemSchema);

export const ResetPasswordResponseSchema = z.object({
  password: z.string(),
});

export const ChangePasswordRequestSchema = z.object({
  current_password: z.string().min(1, "current password is required"),
  new_password: z.string().min(8, "new password must be at least 8 characters"),
});

export const CreateAuthUserRequestSchema = z.object({
  username: z.string().min(3, "username must be at least 3 characters"),
  password: z.string().min(8, "password must be at least 8 characters").optional(),
  role: z.enum(["admin", "dev", "viewer"]),
  databases: z.array(z.string()).optional(),
});

export const TableInfoSchema = z.object({
  name: z.string(),
  rowCount: z.number(),
});

export type TableInfo = z.infer<typeof TableInfoSchema>;

export const ColumnInfoSchema = z.object({
  name: z.string(),
  type: z.string(),
  nullable: z.boolean(),
  default: z.string().nullable(),
  isPrimaryKey: z.boolean(),
});

export type ColumnInfo = z.infer<typeof ColumnInfoSchema>;

export const DataResultSchema = z.object({
  columns: z.array(z.string()),
  rows: z.array(z.array(z.any())),
  total: z.number(),
});

export type DataResult = z.infer<typeof DataResultSchema>;

export const WhereConditionSchema = z.object({
  column: z.string(),
  operator: z.enum(["=", "!=", ">", "<", ">=", "<=", "LIKE", "IS NULL", "IS NOT NULL"]),
  value: z.any().optional(),
});

export type WhereCondition = z.infer<typeof WhereConditionSchema>;

export const ColumnDefSchema = z.object({
  name: z.string(),
  type: z.string(),
  nullable: z.boolean(),
  default: z.string().optional(),
  isPrimaryKey: z.boolean(),
});

export type ColumnDef = z.infer<typeof ColumnDefSchema>;

export const QueryResultSchema = z.object({
  columns: z.array(z.string()),
  rows: z.array(z.array(z.any())),
  rowCount: z.number(),
  duration: z.number(),
  error: z.string().optional(),
});

export type QueryResult = z.infer<typeof QueryResultSchema>;

export const AuditLogEntrySchema = z.object({
  id: z.number(),
  username: z.string(),
  action: z.string(),
  database: z.string(),
  tableName: z.string().nullable().optional(),
  detail: z.any().nullable().optional(),
  ipAddress: z.string().nullable().optional(),
  createdAt: z.string(),
});

export type AuditLogEntry = z.infer<typeof AuditLogEntrySchema>;

export const AuditLogResponseSchema = z.object({
  entries: z.array(AuditLogEntrySchema),
  total: z.number(),
});

export type AuditLogResponse = z.infer<typeof AuditLogResponseSchema>;
