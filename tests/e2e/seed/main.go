// Test-only fixture generator. Never linked into the application.
package main

import (
	"context"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"golang.org/x/crypto/bcrypt"
	"log"
	"os"
)

func main() {
	s, e := storage.Open(context.Background(), os.Args[1])
	if e != nil {
		log.Fatal(e)
	}
	defer s.Close()
	h, e := bcrypt.GenerateFromPassword([]byte("Testing-password-123!"), bcrypt.MinCost)
	if e != nil {
		log.Fatal(e)
	}
	for _, role := range []string{"admin", "user"} {
		login := role
		if role == "user" {
			login = "reader"
		}
		_, e = s.Write.Exec("INSERT INTO users(id,login,name,password_hash,role,must_change) VALUES(?,?,?,?,?,0)", domain.NewID(), login, "Тестовый "+login, h, role)
		if e != nil {
			log.Fatal(e)
		}
	}
	if _, e = s.Write.Exec("INSERT INTO categories(id,title) VALUES(?,?)", domain.NewID(), "Инструкции"); e != nil {
		log.Fatal(e)
	}
}
