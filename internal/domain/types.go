package domain

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
)

type ID string
type Version int64

func NewID() ID { return ID(uuid.NewString()) }

var (
	ErrConflict  = errors.New("another user has changed this content")
	ErrForbidden = errors.New("access denied")
	ErrNotFound  = errors.New("not found")
	ErrBusy      = errors.New("try again shortly")
	ErrInvalid   = errors.New("invalid input")
)

type User struct {
	ID                           ID
	Login, Name, Role            string
	Disabled, MustChangePassword bool
}
type Category struct {
	ID      ID
	Title   string
	Order   int
	Version Version
}
type Article struct {
	ID, CategoryID, AuthorID, LastEditorID ID
	Title                                  string
	Document                               json.RawMessage
	PlainText                              string
	Version                                Version
	Archived                               bool
	PublishedAt, UpdatedAt                 int64
	AuthorName, EditorName, CategoryTitle  string
}
type Draft struct {
	ID, ArticleID, OwnerID, CategoryID ID
	Title                              string
	Document                           json.RawMessage
	Version, BaseArticleVersion        Version
	UpdatedAt                          int64
}
type Media struct {
	ID            ID
	Type          string
	Size          int64
	Width, Height int
}
