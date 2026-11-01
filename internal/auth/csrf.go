package auth

import (
	"crypto/rand"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"os"
	"path/filepath"
)

func loadKey(dir string) ([]byte, error) {
	lock, e := storage.AcquireFileLock(dir, "csrf.lock", true, false)
	if e != nil {
		return nil, e
	}
	defer lock.Close()
	p := filepath.Join(dir, "csrf.key")
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		b = make([]byte, 32)
		if _, e = rand.Read(b); e != nil {
			return nil, e
		}
		e = os.WriteFile(p, b, 0600)
	}
	if e != nil {
		return nil, e
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("invalid CSRF key file")
	}
	return b, nil
}
