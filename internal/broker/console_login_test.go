package broker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestConsoleLoginUsesInternalIdentityAndNeverEchoesPassword(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	gateway := &fakeGateway{}
	var out bytes.Buffer
	logins, logouts := 0, 0
	shell, err := NewAuthenticatedConsoleShell(gateway, catalog, strings.NewReader("alice\rGoodPassword42\rwhoami\rexit\r"), &out, nil, func(_ context.Context, user string, password []byte) (Principal, time.Time, error) {
		logins++
		if user != "alice" || string(password) != "GoodPassword42" {
			t.Fatal("wrong credentials")
		}
		return Principal{UserID: "alice", Roles: []string{"viewer"}, BrokerType: "console", ConsoleSession: strings.Repeat("a", 64)}, time.Now().Add(time.Minute), nil
	}, func(_ context.Context, p Principal) error { logouts++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = shell.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if logins != 1 || logouts != 1 || !strings.Contains(out.String(), "Authenticated as alice") || !strings.Contains(out.String(), "user: alice") {
		t.Fatalf("logins=%d logouts=%d output=%q", logins, logouts, out.String())
	}
	if strings.Contains(out.String(), "GoodPassword42") || strings.Contains(out.String(), strings.Repeat("a", 64)) {
		t.Fatal("credential or session token echoed")
	}
	for _, line := range shell.history {
		if strings.Contains(line, "alice") || strings.Contains(line, "GoodPassword42") {
			t.Fatal("credentials entered history")
		}
	}
}
func TestConsoleNeverDispatchesBeforeSuccessfulLogin(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	gateway := &fakeGateway{}
	var out bytes.Buffer
	attempts := 0
	shell, _ := NewAuthenticatedConsoleShell(gateway, catalog, strings.NewReader("alice\rbad\ralice\rbad\ralice\rbad\rexecute system\r"), &out, nil, func(context.Context, string, []byte) (Principal, time.Time, error) {
		attempts++
		return Principal{}, time.Time{}, errors.New("denied")
	}, func(context.Context, Principal) error { return nil })
	if err := shell.Run(context.Background()); err == nil {
		t.Fatal("failed login accepted")
	}
	if attempts != 3 || len(gateway.requests) != 0 || strings.Contains(out.String(), "Authenticated as") {
		t.Fatal("unauthenticated command executed")
	}
}
