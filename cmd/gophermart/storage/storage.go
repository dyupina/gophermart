package storage

import (
	"database/sql"
	"embed"
	"errors"
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/models"
	"log"
	"time"

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
	AddOrder(userLogin string, orderNumber int) (isAddedToDB bool, err error)
	GetOrders(userLogin string) ([]models.Order, error)
	UpdateOrder(orderNumber int, status string, accrual float64) error
	GetUserBalance(userLogin string) (models.UserBalance, error)
	UpdateUserBalance(userLogin string, orderNumber int, accrualToAdd float64) error
	WithdrawFromUserBalance(userLogin string, orderNumber int, amount float64) error
	GetUserWithdrawals(userLogin string) ([]models.Withdrawal, error)
}

type StorageDB struct {
	DBConn *sql.DB
}

var (
	ErrAddOrderConflict  = errors.New("error AddOrder Conflict")
	ErrInsufficientFunds = errors.New("error insufficient funds")
	ErrGetUserBalance    = errors.New("error in getting balance")
	ErrOpenDBConnection  = errors.New("error opening database connection")
	ErrConnecting        = errors.New("error connecting to database")
	ErrTransaction       = errors.New("error transaction")
)

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

func NewStorage(c *config.Config) (*StorageDB, error) {
	dbConn, err := sql.Open("pgx", c.DBConnection)
	if err != nil {
		return nil, ErrOpenDBConnection
	}

	if err := dbConn.Ping(); err != nil {
		return nil, ErrConnecting
	}

	UpDBMigrations(dbConn)

	return &StorageDB{
		DBConn: dbConn,
	}, nil
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
	_, err3 := s.DBConn.Exec("INSERT INTO users_balances (login) VALUES ($1)", login)

	return err1 == nil && err3 == nil
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

var selectLoginFromUsersOrders = "SELECT login FROM orders WHERE number = $1 AND login != $2"
var isOrderNumberExistsForLogin = "SELECT 1 FROM orders WHERE login = $1 AND number = $2"
var insertNewOrder = "INSERT INTO orders (login, number, status, uploaded_at) VALUES ($1, $2, $3, $4)"

func (s *StorageDB) AddOrder(userLogin string, orderNumber int) (isAddedToDB bool, err error) {
	isAddedToDB = false

	// проверяем есть ли заказ orderNumber у другого пользователя
	row := s.DBConn.QueryRow(selectLoginFromUsersOrders, orderNumber, userLogin)
	if err := row.Scan(new(string)); !errors.Is(err, sql.ErrNoRows) {
		if err == nil {
			return isAddedToDB, ErrAddOrderConflict // StatusConflict
		}
		return isAddedToDB, err
	}

	// существует ли запись с orderNumber в orders для заданного userLogin
	row = s.DBConn.QueryRow(isOrderNumberExistsForLogin, userLogin, orderNumber)
	if err := row.Scan(new(int)); !errors.Is(err, sql.ErrNoRows) {
		if err == nil {
			return isAddedToDB, nil // StatusOK номер заказа уже был загружен этим пользователем
		}
		return isAddedToDB, err
	}

	// Сохранить в таблице orders
	_, err = s.DBConn.Exec(insertNewOrder, userLogin, orderNumber, "NEW", time.Now())
	if err != nil {
		return isAddedToDB, err
	}

	isAddedToDB = true
	return isAddedToDB, nil // StatusAccepted новый номер заказа принят в обработку
}

func (s *StorageDB) GetOrders(userLogin string) ([]models.Order, error) {
	rows, err := s.DBConn.Query(`
		SELECT number, status, accrual, uploaded_at
        FROM orders
        WHERE login = $1
        ORDER BY uploaded_at DESC
    `, userLogin)
	// DESC - в порядке убывания

	if err != nil && rows.Err() != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		var uploadedAt time.Time

		err := rows.Scan(&order.Number, &order.Status, &order.Accrual, &uploadedAt)
		if err != nil {
			return nil, err
		}
		order.UploadedAt = time.Now()
		orders = append(orders, order)
	}

	return orders, nil
}

var updateOrder = "UPDATE orders SET status = $1, accrual = $2, accrual_added = (TRUE) WHERE number = $3"

func (s *StorageDB) UpdateOrder(orderNumber int, status string, accrual float64) error {
	_, err := s.DBConn.Exec(updateOrder, status, accrual, orderNumber)
	return err
}

var getUserBalance = "SELECT current, withdrawn FROM users_balances WHERE login = $1"

func (s *StorageDB) GetUserBalance(userLogin string) (models.UserBalance, error) {
	var balance models.UserBalance
	err := s.DBConn.QueryRow(getUserBalance, userLogin).Scan(&balance.Current, &balance.Withdrawn)

	if err != nil {
		return models.UserBalance{}, ErrGetUserBalance
	}

	return balance, nil
}

func (s *StorageDB) UpdateUserBalance(userLogin string, orderNumber int, accrualToAdd float64) error {
	tx, err := s.DBConn.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("rollback error: %v", err)
		}
	}()

	// Обновляем баланс для заказа пользователя, только если у заказа accrual_added == FALSE
	_, err = tx.Exec(`UPDATE users_balances
		SET current = current + $1
		FROM orders o
		WHERE users_balances.login = (SELECT login FROM users WHERE users.login = $2)
		AND o.accrual_added = FALSE`, accrualToAdd, userLogin)

	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return ErrTransaction
	}

	return nil
}

var getCurrentBalance = "SELECT current FROM users_balances WHERE login = $1"
var updateMoney = "UPDATE users_balances SET current = current - $1, withdrawn = withdrawn + $1 WHERE login = $2"

func (s *StorageDB) WithdrawFromUserBalance(userLogin string, orderNumber int, amount float64) error {
	var currentBalance float64

	tx, err := s.DBConn.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("rollback error: %v", err)
		}
	}()

	err = tx.QueryRow(getCurrentBalance, userLogin).Scan(&currentBalance)
	if err != nil {
		return err
	}

	if currentBalance < amount {
		return ErrInsufficientFunds
	}
	_, err = tx.Exec(updateMoney, amount, userLogin)
	if err != nil {
		return err
	}

	// Добавляем _каждую_ операцию списания
	_, err = s.DBConn.Exec(`
		INSERT INTO users_withdrawals (login, order_number, sum, processed_at)
		VALUES ($1, $2, $3, $4)`, userLogin, orderNumber, amount, time.Now())

	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return ErrTransaction
	}

	return nil
}

func (s *StorageDB) GetUserWithdrawals(userLogin string) ([]models.Withdrawal, error) {
	rows, err := s.DBConn.Query(`
        SELECT order_number, sum, processed_at 
        FROM users_withdrawals 
        WHERE login = $1 
        ORDER BY processed_at DESC`, userLogin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []models.Withdrawal
	for rows.Next() {
		var w models.Withdrawal
		if err := rows.Scan(&w.Order, &w.Sum, &w.ProcessedAt); err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return withdrawals, nil
}
