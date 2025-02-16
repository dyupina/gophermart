package storage

import (
	"database/sql"
	"embed"
	"gophermart/cmd/gophermart/config"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"
)

type StorageService interface {
	HashPassword(password string) (string, error)
	CheckPasswordHash(password, hash string) bool
	SaveLoginPassword(uid, login, hashedPassword string) bool
}

type StorageDB struct {
	DBConn *sql.DB
}

//go:embed db/migrations/*.sql
var embedMigrations embed.FS

func UpDBMigrations(db *sql.DB) {
	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		log.Printf("error setting SQL dialect\n")
	}

	if err := goose.Up(db, "db/migrations"); err != nil {
		log.Printf("error migration %s\n", err.Error())
	}
}

func NewStorage(c *config.Config) StorageService {
	DBConn, err := sql.Open("pgx", c.DBConnection)
	if err != nil {
		log.Printf("Error opening database connection: %v\n", err)
		return nil
	}

	if err := DBConn.Ping(); err != nil {
		log.Printf("Error connecting to database: %v\n", err)
		return nil
	}

	UpDBMigrations(DBConn)

	return &StorageDB{
		DBConn: DBConn,
	}
}

func (s *StorageDB) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func (s *StorageDB) CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (s *StorageDB) SaveLoginPassword(uid, login, hashedPassword string) bool {
	_, err := s.DBConn.Exec("INSERT INTO users (uid, login, password) VALUES ($1, $2, $3)", uid, login, hashedPassword)
	return err == nil
}
