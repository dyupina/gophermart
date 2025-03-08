//go:build unit
// +build unit

package storage_test

import (
	"database/sql"
	"gophermart/cmd/gophermart/models"
	"gophermart/cmd/gophermart/storage"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_HashPassword(t *testing.T) {
	storage := &storage.StorageUtils_{}
	password := "secret"
	hashedPassword, err := storage.HashPassword(password)

	require.NoError(t, err)
	assert.NotEqual(t, "", hashedPassword)
	assert.True(t, storage.CheckPasswordHash(password, hashedPassword))
}

func Test_SaveLoginPassword(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &storage.StorageDB{DBConn: db}

	login := "testuser"
	hashedPassword := "testuser_hashed"

	mock.ExpectExec("INSERT INTO users").
		WithArgs(login, hashedPassword).
		WillReturnResult(sqlmock.NewResult(1, 1))

	ok := storage.SaveLoginPassword(login, hashedPassword)

	assert.True(t, ok)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func Test_GetHashedPasswordByLogin(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &storage.StorageDB{DBConn: db}

	login := "testuser"
	expectedPassword := "testuser_hashed"
	mock.ExpectQuery("SELECT password FROM users").
		WithArgs(login).
		WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(expectedPassword))

	hashedPassword := storage.GetHashedPasswordByLogin(login)

	assert.Equal(t, expectedPassword, hashedPassword)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func Test_SaveUID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &storage.StorageDB{DBConn: db}

	userID := "12345"
	login := "testuser"

	mock.ExpectExec("UPDATE users SET uid = \\$1 WHERE login = \\$2").
		WithArgs(userID, login).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = storage.SaveUID(userID, login)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func Test_GetLoginByUID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &storage.StorageDB{DBConn: db}

	userID := "12345"
	expectedLogin := "testuser"

	mock.ExpectQuery("SELECT login FROM users WHERE uid=\\$1").
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"login"}).AddRow(expectedLogin))

	login := storage.GetLoginByUID(userID)

	assert.Equal(t, expectedLogin, login)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func Test_AddOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &storage.StorageDB{DBConn: db}

	userLogin := "testuser"
	orderNumber := 123

	mock.ExpectQuery("SELECT login FROM orders WHERE number = \\$1 AND login != \\$2").
		WithArgs(orderNumber, userLogin).
		WillReturnRows(sqlmock.NewRows(nil)) // без конфликтов

	mock.ExpectQuery("SELECT 1 FROM orders WHERE login = \\$1 AND number = \\$2").
		WithArgs(userLogin, orderNumber).
		WillReturnRows(sqlmock.NewRows(nil)) // заказ отсутствует

	mock.ExpectExec("INSERT INTO orders \\(login, number, status, uploaded_at\\) VALUES \\(\\$1, \\$2, \\$3, \\$4\\)").
		WithArgs(userLogin, orderNumber, "NEW", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1)) // успешное добавление

	isAdded, err := storage.AddOrder(userLogin, orderNumber)

	assert.True(t, isAdded)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func Test_GetOrders(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &storage.StorageDB{DBConn: db}

	userLogin := "testuser"

	rows := sqlmock.NewRows([]string{"number", "status", "accrual", "uploaded_at"}).
		AddRow("12345", "NEW", 10.5, time.Now()).
		AddRow("67890", "PROCESSED", 15.0, time.Now())

	mock.ExpectQuery(`SELECT number, status, accrual, uploaded_at FROM orders WHERE login = \$1 ORDER BY uploaded_at DESC`).
		WithArgs(userLogin).
		WillReturnRows(rows)

	orders, err := storage.GetOrders(userLogin)

	assert.NoError(t, err)
	require.Len(t, orders, 2)

	assert.Equal(t, "12345", orders[0].Number)
	assert.Equal(t, "NEW", orders[0].Status)
	assert.Equal(t, 10.5, orders[0].Accrual)

	assert.Equal(t, "67890", orders[1].Number)
	assert.Equal(t, "PROCESSED", orders[1].Status)
	assert.Equal(t, 15.0, orders[1].Accrual)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func Test_UpdateOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &storage.StorageDB{DBConn: db}

	orderNumber := 12345
	status := "PROCESSED"
	accrual := 100.50

	mock.ExpectExec("UPDATE orders SET status = \\$1, accrual = \\$2, accrual_added = \\(TRUE\\) WHERE number = \\$3").
		WithArgs(status, accrual, orderNumber).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = storage.UpdateOrder(orderNumber, status, accrual)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func Test_GetUserBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &storage.StorageDB{DBConn: db}

	userLogin := "testuser"
	expectedBalance := models.UserBalance{
		Current:   500.00,
		Withdrawn: 200.00,
	}

	// вместо BalanceForUserLogin
	mock.ExpectQuery("SELECT 1 FROM users_balances WHERE login=\\$1").
		WithArgs("testuser").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO users_balances \\(login\\) VALUES \\(\\$1\\)").
		WithArgs("testuser").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery("SELECT current, withdrawn FROM users_balances WHERE login = \\$1").
		WithArgs(userLogin).
		WillReturnRows(sqlmock.NewRows([]string{"current", "withdrawn"}).
			AddRow(expectedBalance.Current, expectedBalance.Withdrawn))

	balance, err := storage.GetUserBalance(userLogin)

	assert.NoError(t, err)
	assert.Equal(t, expectedBalance, balance)
	assert.NoError(t, mock.ExpectationsWereMet())
}
