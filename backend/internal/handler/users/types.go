package users

type UserRecord struct {
	Username   string   `json:"username"`
	Databases  []string `json:"databases"`
	Access     string   `json:"access"`
	AllowedIps []string `json:"allowedIps"`
	CreatedAt  string   `json:"createdAt"`
}

type CreateUserResponse struct {
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	Databases        []string `json:"databases"`
	ConnectionString string   `json:"connectionString"`
	Access           string   `json:"access"`
	AllowedIps       []string `json:"allowedIps"`
	CreatedAt        string   `json:"createdAt"`
}

type CreateUserRequest struct {
	Databases  []string `json:"databases"`
	Username   string   `json:"username"`
	Access     string   `json:"access"`
	Password   string   `json:"password,omitempty"`
	AllowedIps []string `json:"allowedIps,omitempty"`
}

type UpdateUserRequest struct {
	Password         string   `json:"password,omitempty"`
	Access           string   `json:"access,omitempty"`
	GeneratePassword bool     `json:"generatePassword,omitempty"`
	AllowedIps       []string `json:"allowedIps,omitempty"`
	Databases        []string `json:"databases,omitempty"`
}

type AddDatabaseRequest struct {
	Database string `json:"database"`
}
