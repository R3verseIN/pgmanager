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
