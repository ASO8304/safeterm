package broker

import (
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
	"strings"
	"testing"
)

func TestPasswordAuthenticationBindsIdentityAndRejectsInvalidCredentials(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), PasswordHashCost)
	if err != nil {
		t.Fatal(err)
	}
	key := testSigner(t)
	a, err := NewSSHAuthenticator(SSHIdentityConfig{Users: []SSHUser{
		{UserID: "alice", Roles: []string{"viewer"}, PasswordHash: string(hash)},
		{UserID: "keyonly", Roles: []string{"viewer"}, AuthorizedKeys: []string{string(ssh.MarshalAuthorizedKey(key.PublicKey()))}},
	}}, map[string]bool{"viewer": true})
	if err != nil {
		t.Fatal(err)
	}
	permissions, err := a.AuthenticatePassword(connectionMetadata{user: "alice"}, []byte("test-password"))
	if err != nil {
		t.Fatal(err)
	}
	principal, err := principalFromPermissions(permissions)
	if err != nil || principal.UserID != "alice" || strings.Join(principal.Roles, ",") != "viewer" {
		t.Fatalf("wrong authenticated principal: %+v %v", principal, err)
	}
	for _, test := range []struct{ user, password string }{
		{"alice", "wrong-password"}, {"missing", "test-password"}, {"keyonly", "test-password"}, {"alice", ""}, {"alice", strings.Repeat("x", 73)},
	} {
		if _, err := a.AuthenticatePassword(connectionMetadata{user: test.user}, []byte(test.password)); err == nil || err.Error() != "broker: password authentication failed" {
			t.Fatalf("credentials were accepted or error leaked details for %s", test.user)
		}
	}
	// Expensive verification slots must reject excess requests without queuing work.
	for i := 0; i < cap(a.passwordSlots); i++ {
		a.passwordSlots <- struct{}{}
	}
	if _, err := a.AuthenticatePassword(connectionMetadata{user: "alice"}, []byte("test-password")); err == nil {
		t.Fatal("password work limit ignored")
	}
	for len(a.passwordSlots) > 0 {
		<-a.passwordSlots
	}
}

func TestPasswordHashesFailClosedEvenWithValidPublicKey(t *testing.T) {
	key := string(ssh.MarshalAuthorizedKey(testSigner(t).PublicKey()))
	for _, hash := range []string{"plaintext-password", "$2a$04$" + strings.Repeat("a", 53), "$2a$31$" + strings.Repeat("a", 53), "$2a$12$short", "$2a$12$" + strings.Repeat("!", 53)} {
		_, err := NewSSHAuthenticator(SSHIdentityConfig{Users: []SSHUser{{UserID: "alice", Roles: []string{"viewer"}, PasswordHash: hash, AuthorizedKeys: []string{key}}}}, map[string]bool{"viewer": true})
		if err == nil {
			t.Fatal("invalid password hash accepted")
		}
		if strings.Contains(err.Error(), hash) {
			t.Fatal("password hash leaked in error")
		}
	}
}
