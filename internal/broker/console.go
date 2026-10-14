package broker

// ConsoleIdentity is an internal SafeTerm Console login. OSUser is accepted
// only to read legacy installations; it never grants authentication.
type ConsoleIdentity struct {
	OSUser       string   `json:"os_user,omitempty"`
	PasswordHash string   `json:"password_hash,omitempty"`
	Disabled     bool     `json:"disabled,omitempty"`
	UserID       string   `json:"user_id"`
	Roles        []string `json:"roles"`
}

type ConsoleIdentityConfig struct {
	Users []ConsoleIdentity `json:"users"`
}

// ReadConsoleUsersData is for the isolated Identity daemon, never the shared
// console process. Reuse the security-file checks before parsing credentials.
func ReadConsoleUsersData(path string) ([]byte, error) {
	return readSecurityFile(path, "console identity file", maxIdentityFileBytes, 0o027)
}
