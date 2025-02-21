package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"gophermart/cmd/gophermart/config"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"
)

type StorageService interface {
	HashPassword(password string) (string, error)
	CheckPasswordHash(password, hash string) bool
	SaveLoginPassword(login, hashedPassword string) bool
	GetHashedPasswordByLogin(login string) string
	SaveUID(userID, login string) error
	GetLoginByUID(userID string) string
	AddOrder(userLogin, orderNumber string) (bool, error)
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

func (s *StorageDB) SaveLoginPassword(login, hashedPassword string) bool {
	_, err1 := s.DBConn.Exec("INSERT INTO users (login, password) VALUES ($1, $2)", login, hashedPassword)
	_, err2 := s.DBConn.Exec("INSERT INTO users_orders (login, orders) VALUES ($1, '{}')", login)
	return err1 == nil && err2 == nil
}

func (s *StorageDB) GetHashedPasswordByLogin(login string) string {
	var hashedPassword string
	_ = s.DBConn.QueryRow("SELECT password FROM users WHERE login=$1", login).Scan(&hashedPassword)
	return hashedPassword
}

func (s *StorageDB) SaveUID(userID, login string) error {
	_, err := s.DBConn.Exec("UPDATE users SET uid = $1 WHERE login = $2", userID, login)
	return err
}

func (s *StorageDB) GetLoginByUID(userID string) string {
	var login string
	_ = s.DBConn.QueryRow("SELECT login FROM users WHERE uid=$1", userID).Scan(&login)
	return login
}

// isNew, isOwnedByUser
func (s *StorageDB) AddOrder(userLogin, orderNumber string) (bool, error) {
	row := s.DBConn.QueryRow("SELECT login FROM users_orders WHERE $1 = ANY(orders) AND login != $2", orderNumber, userLogin)
	if err := row.Scan(new(string)); err != sql.ErrNoRows {
		if err == nil {
			return false, fmt.Errorf("conflict") // StatusConflict
		}
		return false, err
	}

	row = s.DBConn.QueryRow("SELECT 1 FROM users_orders WHERE login = $1 AND $2 = ANY(orders)", userLogin, orderNumber)
	if err := row.Scan(new(int)); err != sql.ErrNoRows {
		if err == nil {
			return false, nil // StatusOK номер заказа уже был загружен этим пользователем
		}
		return false, err
	}

	_, err := s.DBConn.Exec("UPDATE users_orders SET orders = array_append(orders, $2) WHERE login = $1", userLogin, orderNumber)
	if err != nil {
		return false, err
	}

	return true, nil // StatusAccepted новый номер заказа принят в обработку
}
