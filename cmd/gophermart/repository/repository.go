package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/9ssi7/nanoid"
)

var ErrDuplicateURL = errors.New("duplicate URL")

type Repository interface {
	GetShortURL_db(originalURL string) (string, error)
}
type Repo struct {
}

const insertRow = `
INSERT INTO urls (user_id, short_url, original_url) VALUES ($1, $2, $3)
ON CONFLICT (original_url) DO NOTHING
RETURNING short_url`

func (s *Repo) GetShortURLDB(userID, originalURL string, db *sql.DB) (string, error) {
	var shortURL string
	var retErr error
	shortID := GenerateShortID()

	row := db.QueryRow(insertRow, userID, shortID, originalURL)
	_ = row.Scan(&shortURL)

	retErr = nil

	if shortURL == "" {
		// Получение существующего сокращенного URL
		row := db.QueryRow(
			"SELECT short_url FROM urls WHERE original_url = $1", originalURL)
		err := row.Scan(&shortURL)
		if err != nil {
			return "", fmt.Errorf("error select query: %v", err)
		}
		retErr = ErrDuplicateURL
		fmt.Println("Existing short URL:", shortURL)
	}

	return shortURL, retErr
}

func GenerateShortID() string {
	id, _ := nanoid.New()
	return id
}
