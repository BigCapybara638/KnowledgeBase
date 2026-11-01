// Package testutil contains fixtures for integration tests, not runtime seeding.
package testutil

import (
	"context"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"golang.org/x/crypto/bcrypt"
	"testing"
)

const Password = "Testing-password-123!"

func Store(t testing.TB) *storage.Store {
	t.Helper()
	s, e := storage.Open(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func User(t testing.TB, s *storage.Store, role string) domain.ID {
	t.Helper()
	id := domain.NewID()
	h, e := bcrypt.GenerateFromPassword([]byte(Password), bcrypt.MinCost)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Write.Exec("INSERT INTO users(id,login,name,password_hash,role,must_change) VALUES(?,?,?,?,?,0)", id, string(id), "Сотрудник", h, role)
	if e != nil {
		t.Fatal(e)
	}
	return id
}
