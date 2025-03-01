package storage

import (
	"database/sql"
	"embed"
	"fmt"
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
	AddOrder(userLogin string, orderNumber int) (bool, error)
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
	_, err3 := s.DBConn.Exec("INSERT INTO users_balances (login) VALUES ($1)", login)

	return err1 == nil && err2 == nil && err3 == nil
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

func (s *StorageDB) AddOrder(userLogin string, orderNumber int) (bool, error) {
	row := s.DBConn.QueryRow("SELECT login FROM users_orders WHERE $1 = ANY(orders) AND login != $2", orderNumber, userLogin)
	if err := row.Scan(new(string)); err != sql.ErrNoRows {
		if err == nil {
			return false, fmt.Errorf("conflict") // StatusConflict
		}
		return false, err
	}

	// существует ли запись с orderNumber в users_orders
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

	// Сохранить в таблице orders
	_, err = s.DBConn.Exec(
		"INSERT INTO orders (number, status, uploaded_at) VALUES ($1, $2, $3)",
		orderNumber, "NEW", time.Now())
	if err != nil {
		return false, err
	}

	return true, nil // StatusAccepted новый номер заказа принят в обработку
}

func (s *StorageDB) GetOrders(userLogin string) ([]models.Order, error) {
	rows, err := s.DBConn.Query(`
		SELECT o.number, o.status, o.accrual, o.uploaded_at
        FROM orders o
        JOIN users_orders uo ON o.number = ANY(uo.orders)
        WHERE uo.login = $1
        ORDER BY o.uploaded_at DESC
    `, userLogin)
	// DESC - в порядке убывания

	if err != nil {
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
		order.UploadedAt = uploadedAt.Format(time.RFC3339)
		orders = append(orders, order)
	}

	return orders, nil
}

func (s *StorageDB) UpdateOrder(orderNumber int, status string, accrual float64) error {
	tx, err := s.DBConn.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("rollback error: %v", err)
		}
	}()

	_, err = tx.Exec("UPDATE orders SET status = $1, accrual = $2, accrual_added = (TRUE) WHERE number = $3", status, accrual, orderNumber)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *StorageDB) GetUserBalance(userLogin string) (models.UserBalance, error) {
	var balance models.UserBalance
	err := s.DBConn.QueryRow(`SELECT current, withdrawn 
	FROM users_balances WHERE login = $1`, userLogin).Scan(&balance.Current, &balance.Withdrawn)

	return balance, err
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

	return tx.Commit()
}

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

	err = tx.QueryRow("SELECT current FROM users_balances WHERE login = $1", userLogin).Scan(&currentBalance)
	if err != nil {
		return err
	}

	if currentBalance < amount {
		return fmt.Errorf("insufficient funds")
	}

	_, err = tx.Exec("UPDATE users_balances SET current = current - $1, withdrawn = withdrawn + $1 WHERE login = $2", amount, userLogin)
	if err != nil {
		return err
	}

	// Добавить запись о списании средств, если для order_number ее еще не было. И если была, то обновить
	// EXCLUDED - это специальная псевдонимная таблица, которая ссылается на предполагаемое новое значение в случае конфликта
	_, err = s.DBConn.Exec(`
		INSERT INTO users_withdrawals (login, order_number, sum, processed_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (order_number) DO UPDATE 
		SET sum = users_withdrawals.sum + EXCLUDED.sum, 
			processed_at = EXCLUDED.processed_at`, userLogin, orderNumber, amount, time.Now())

	if err != nil {
		return err
	}

	return tx.Commit()
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
