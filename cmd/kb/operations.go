package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/backup"
	"github.com/BigCapybara638/KnowledgeBase/internal/media"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"os"
	"path/filepath"
	"time"
)

func dataDirectory() string {
	if s := os.Getenv("KB_DATA_DIR"); s != "" {
		return s
	}
	return "./data"
}
func operations(command string, args []string) error {
	ctx := context.Background()
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	switch command {
	case "backup":
		root := os.Getenv("KB_BACKUP_DIR")
		if root == "" {
			root = filepath.Join(dataDirectory(), "backups")
		}
		out := f.String("output", root, "backup directory")
		if e := f.Parse(args); e != nil {
			return e
		}
		m, e := backup.Create(ctx, dataDirectory(), *out)
		if e != nil {
			return e
		}
		fmt.Println("Backup created:", m.Directory)
		return nil
	case "restore":
		source := f.String("from", "", "backup bundle")
		target := f.String("target", dataDirectory(), "empty target directory")
		if e := f.Parse(args); e != nil {
			return e
		}
		if *source == "" {
			return fmt.Errorf("--from is required")
		}
		if e := backup.Restore(ctx, *source, *target); e != nil {
			return e
		}
		fmt.Println("Restored into:", *target)
		return nil
	case "maintenance":
		if e := f.Parse(args); e != nil {
			return e
		}
		s, e := storage.Open(ctx, dataDirectory())
		if e != nil {
			return e
		}
		defer s.Close()
		n, e := media.New(s).CollectOrphans(ctx, time.Now().Add(-24*time.Hour))
		if e != nil {
			return e
		}
		e = s.WithWrite(ctx, func(tx *sql.Tx) error {
			if _, e := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<?", time.Now().Unix()); e != nil {
				return e
			}
			_, e := tx.ExecContext(ctx, "DELETE FROM login_limits WHERE window_start<?", time.Now().Add(-24*time.Hour).Unix())
			return e
		})
		if e != nil {
			return e
		}
		fmt.Println("Unreferenced images removed:", n)
		return nil
	}
	return fmt.Errorf("unknown operation")
}
