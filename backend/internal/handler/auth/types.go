package auth

type SetupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type CreateAuthUserRequest struct {
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	Role      string   `json:"role"`
	Databases []string `json:"databases"`
}

type UpdateAuthUserRequest struct {
	Role      string   `json:"role,omitempty"`
	Databases []string `json:"databases"`
}

type AuthUserListItem struct {
	ID        int      `json:"id"`
	Username  string   `json:"username"`
	Role      string   `json:"role"`
	Databases []string `json:"databases,omitempty"`
	CreatedAt string   `json:"createdAt"`
}
