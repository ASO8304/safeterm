package broker

import "testing"

func TestPasswordProvisioningRejectsCharactersTheTerminalCannotEnter(t *testing.T) {
	for _, password := range []string{"GoodPass42", "GoodPass42! @"} {
		if err := ValidatePasswordComplexity([]byte(password)); err != nil {
			t.Fatal(err)
		}
	}
	for _, password := range []string{"GoodPass42é", "GoodPass42\n", "short1A", "lowercase123", "UPPERCASE123", "NoNumberHere"} {
		if ValidatePasswordComplexity([]byte(password)) == nil {
			t.Fatal("unusable or weak password accepted")
		}
	}
}
