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
  createdAt: z.string(),
});

export const UpdateUserRequestSchema = z.object({
  password: z.string().optional(),
  access: z.enum(["read", "write", "ddl", "full"]).optional(),
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
  password: z.string().min(8, "password must be at least 8 characters"),
  role: z.enum(["admin", "dev", "viewer"]),
  databases: z.array(z.string()).optional(),
});
