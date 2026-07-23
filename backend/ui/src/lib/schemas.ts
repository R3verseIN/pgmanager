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
