package broker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
)

var ErrPasswordChangedSignIn = errors.New("password changed; sign in again")

type PasswordChanger func(context.Context, Principal, []byte, []byte) (Principal, error)

func (s *Shell) SetPasswordChanger(change PasswordChanger)     { s.passwordChange = change }
func (s *SSHServer) SetPasswordChanger(change PasswordChanger) { s.passwordChange = change }

func (s *Shell) changePassword(ctx context.Context, fields []string, input <-chan byte) (bool, error) {
	if !s.atRoot() {
		return false, fmt.Errorf("passwd is available at the main prompt; use exit to move up")
	}
	if len(fields) == 2 && fields[1] == "?" {
		s.helpRow("<cr>", "Change your current login's password; hidden current/new/confirmation prompts")
		return false, nil
	}
	if len(fields) != 1 {
		return false, fmt.Errorf("usage: passwd; passwords must be entered only at the hidden prompts")
	}
	if len(s.stagedLogical) > 0 {
		return false, fmt.Errorf("save or cancel staged configuration before changing your password")
	}
	if s.passwordChange == nil || input == nil {
		return false, fmt.Errorf("password changes are unavailable in this session")
	}
	read := func(prompt string) ([]byte, error) {
		value, err := s.readLine(ctx, input, s.styled(styleCommand, prompt), true, false)
		return []byte(value), err
	}
	current, err := read("Current password: ")
	defer clear(current)
	if err != nil {
		return false, err
	}
	next, err := read("New password: ")
	defer clear(next)
	if err != nil {
		return false, err
	}
	if err := ValidatePasswordComplexity(next); err != nil {
		return false, err
	}
	confirm, err := read("Confirm new password: ")
	defer clear(confirm)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(next, confirm) {
		return false, fmt.Errorf("new passwords do not match; password unchanged")
	}
	principal, err := s.passwordChange(ctx, s.principal, current, next)
	if errors.Is(err, ErrPasswordChangedSignIn) {
		s.writeStyled(styleSuccess, "Password changed. Sign in again to continue.\n")
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if principal.UserID != s.principal.UserID || principal.BrokerType != s.principal.BrokerType {
		s.writeStyled(styleWarning, "Password changed. Sign in again to continue.\n")
		return true, nil
	}
	s.principal = principal
	s.writeStyled(styleSuccess, "Password changed successfully.\n")
	return false, nil
}
