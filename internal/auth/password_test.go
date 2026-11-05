package auth

import (
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
)

func TestPasswordEightCharacters(t *testing.T) {
	for _, password := range []string{"Abcd123!", "Пароль12"} {
		hash, err := passwordHash(password)
		if err != nil {
			t.Fatalf("eight characters rejected: %v", err)
		}
		if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil {
			t.Fatal(err)
		}
	}
	for _, password := range []string{"Abcd12!", "Пароль1", strings.Repeat("я", 37)} {
		if _, err := passwordHash(password); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
}
