package auth

import (
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"golang.org/x/crypto/bcrypt"
	"unicode/utf8"
)

func passwordHash(password string) ([]byte, error) {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || len(password) > 72 {
		return nil, domain.ErrInvalid
	}
	return bcrypt.GenerateFromPassword([]byte(password), 12)
}
