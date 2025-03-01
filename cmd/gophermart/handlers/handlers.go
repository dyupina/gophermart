package handlers

import (
	"encoding/json"
	"fmt"
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/models"
	"gophermart/cmd/gophermart/storage"
	"gophermart/cmd/gophermart/user"
	"gophermart/cmd/gophermart/utils"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

type Controller struct {
	conf           *config.Config
	storageService storage.StorageService
	sugar          *zap.SugaredLogger
	userService    user.UserService
	workerPool     *WorkerPool
	AccrualService utils.AccrualService
}

func NewController(conf *config.Config, storageService storage.StorageService,
	logger *zap.SugaredLogger, us user.UserService, wp *WorkerPool, accrualService utils.AccrualService) *Controller {
	con := &Controller{
		conf:           conf,
		storageService: storageService,
		sugar:          logger,
		userService:    us,
		workerPool:     wp,
		AccrualService: accrualService,
	}

	con.workerPool.Start(con)

	return con
}

func (con *Controller) Register() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")

		var user_ user.User
		err := json.NewDecoder(req.Body).Decode(&user_)
		login := user_.Login
		password := user_.Password
		if err != nil || login == "" || password == "" {
			con.Debug(res, "Bad request", http.StatusBadRequest)
			return
		}

		hashedPassword, err := con.storageService.HashPassword(password)
		if err != nil {
			con.Debug(res, "Internal server error", http.StatusInternalServerError)
			return
		}

		ok := con.storageService.SaveLoginPassword(login, hashedPassword)
		if !ok {
			con.Debug(res, "Conflict: Login already taken", http.StatusConflict)
			return
		}

		_ = con.userService.SetUserIDCookie(res, userID)
		con.Debug(res, "Register success", http.StatusOK)
	}
}

func (con *Controller) Login() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")

		var user_ user.User
		err := json.NewDecoder(req.Body).Decode(&user_)
		if err != nil || user_.Login == "" || user_.Password == "" {
			con.Debug(res, "Bad request", http.StatusBadRequest)
			return
		}

		storedHashedPassword := con.storageService.GetHashedPasswordByLogin(user_.Login)
		if storedHashedPassword == "" || !con.storageService.CheckPasswordHash(user_.Password, storedHashedPassword) {
			con.Debug(res, "Unauthorized: Invalid login/password", http.StatusUnauthorized)
			return
		}

		err = con.storageService.SaveUID(userID, user_.Login)
		if err != nil {
			con.Debug(res, "Bad request", http.StatusBadRequest)
			return
		}
		_ = con.userService.SetUserIDCookie(res, userID)
		con.Debug(res, "Login success", http.StatusOK)
	}
}

func (con *Controller) OrdersUpload() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")
		userLogin := con.storageService.GetLoginByUID(userID)
		if userLogin == "" {
			con.Debug(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if !strings.Contains(req.Header.Get("Content-Type"), "text/plain") {
			con.Debug(res, "Bad Request", http.StatusBadRequest)
			return
		}

		body, _ := io.ReadAll(req.Body)
		defer req.Body.Close()
		orderNumber, _ := strconv.Atoi(string(body))
		if !models.IsValidOrderNumber(strconv.Itoa(orderNumber)) {
			con.Debug(res, "Unprocessable Entity", http.StatusUnprocessableEntity)
			return
		}

		// TEST @@@ Типа совершаем покупку (POST /api/orders)
		con.AccrualService.MakePurchase(orderNumber)

		orderAdded, err := con.storageService.AddOrder(userLogin, orderNumber)
		if err != nil {
			if err.Error() == "conflict" {
				con.Debug(res, "Conflict", http.StatusConflict)
				return
			}
			con.Debug(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// 3. Заказ попадает в систему расчёта баллов лояльности (в Accrual) @@@
		con.workerPool.AddTask(Task{UserLogin: userLogin, OrderNumber: orderNumber})

		_ = con.userService.SetUserIDCookie(res, userID)
		if orderAdded {
			res.WriteHeader(http.StatusAccepted) // Новый номер заказа принят в обработку
		} else {
			con.Debug(res, "POST orders success", http.StatusOK) // Номер заказа уже был загружен этим пользователем
		}
	}
}

func (con *Controller) OrdersGet() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")
		userLogin := con.storageService.GetLoginByUID(userID)
		if userLogin == "" {
			con.Debug(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		orders, err := con.storageService.GetOrders(userLogin)
		if err != nil {
			con.Debug(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if len(orders) == 0 {
			con.Debug(res, "No Content", http.StatusNoContent)
			return
		}

		tasks := make([]Task, len(orders))
		// Обновление статусов заказов через систему расчёта начислений (Accrual) @@@
		for i, order := range orders {
			orderNumber, _ := strconv.Atoi(order.Number)
			tasks[i] = Task{UserLogin: userLogin, OrderNumber: orderNumber}
			con.workerPool.AddTask(tasks[i])
		}

		go func() {
			for range tasks {
				select {
				case result := <-con.workerPool.results:
					for i, order := range orders {
						if order.Number == result.Order {
							orders[i].Status = result.Status
							orders[i].Accrual = result.Accrual
							break
						}
					}
				case errA := <-con.workerPool.errors:
					if errA.Error() == "error UpdateUserBalance" {
						con.Debug(res, "error UpdateUserBalance", http.StatusInternalServerError)
					} else if errA.Error() == "Error UpdateOrder" {
						con.Debug(res, "Error UpdateOrder", http.StatusInternalServerError)
					}
					return
				}
			}
		}()

		res.Header().Set("Content-Type", "application/json")
		_ = con.userService.SetUserIDCookie(res, userID)
		res.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(res).Encode(orders)
	}
}

func (con *Controller) UserBalance() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")
		userLogin := con.storageService.GetLoginByUID(userID)
		if userLogin == "" {
			con.Debug(res, "Unauthorized", http.StatusUnauthorized)
			return
		}
		balance, err := con.storageService.GetUserBalance(userLogin)
		if err != nil {
			con.Debug(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		res.Header().Set("Content-Type", "application/json")
		_ = con.userService.SetUserIDCookie(res, userID)
		res.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(res).Encode(balance)
	}
}

func (con *Controller) RequestForWithdrawal() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")
		userLogin := con.storageService.GetLoginByUID(userID)
		if userLogin == "" {
			con.Debug(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var wr models.WithdrawRequest
		if err := json.NewDecoder(req.Body).Decode(&wr); err != nil {
			con.Debug(res, "Unprocessable Entity", http.StatusUnprocessableEntity)
			return
		}

		orderNumber := wr.Order
		if !models.IsValidOrderNumber(orderNumber) {
			con.Debug(res, "Unprocessable Entity (invalid order number)", http.StatusUnprocessableEntity)
			return
		}

		on, _ := strconv.Atoi(orderNumber)
		err := con.storageService.WithdrawFromUserBalance(userLogin, on, wr.Sum)
		if err != nil {
			if err.Error() == "insufficient funds" {
				con.Debug(res, "Insufficient funds", http.StatusPaymentRequired)
			} else {
				con.Debug(res, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
		_ = con.userService.SetUserIDCookie(res, userID)
		con.Debug(res, "Request for withdrawal success", http.StatusOK)
	}
}

func (con *Controller) InfoAboutWithdrawals() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")
		userLogin := con.storageService.GetLoginByUID(userID)
		if userLogin == "" {
			con.Debug(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		withdrawals, err := con.storageService.GetUserWithdrawals(userLogin)
		if err != nil {
			con.Debug(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if len(withdrawals) == 0 {
			con.Debug(res, "No Withdrawals", http.StatusNoContent)
			return
		}

		res.Header().Set("Content-Type", "application/json")
		_ = con.userService.SetUserIDCookie(res, userID)
		res.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(res).Encode(withdrawals)
	}
}

// Запрос в систему расчёта баллов лояльности (в Accrual) @@@ GET /api/orders/{number}
func (con *Controller) RequestToAccrual(userLogin string, orderNumber int) (*models.AccrualResponse, error) {
	resp, err := con.AccrualService.RequestToAccrualByOrderumber(orderNumber)
	if err != nil {
		fmt.Println("Error sending GET request:", err)
		return nil, err
	}

	var accrualResponse *models.AccrualResponse
	if resp.StatusCode() == http.StatusOK {
		if err := json.Unmarshal(resp.Body(), &accrualResponse); err != nil {
			return nil, err
		}
		status := accrualResponse.Status
		accrual := accrualResponse.Accrual

		// Обновить данные о бонусах в таблице users_balances
		if err = con.storageService.UpdateUserBalance(userLogin, orderNumber, accrual); err != nil {
			return nil, fmt.Errorf("error UpdateUserBalance")
		}

		// Обновить данные о заказе в таблице orders
		if err = con.storageService.UpdateOrder(orderNumber, status, accrual); err != nil {
			return nil, fmt.Errorf("error UpdateOrder")
		}
	} else if resp.StatusCode() == http.StatusTooManyRequests {
		retryAfter := resp.Header().Get("Retry-After")
		retryAfterDuration, err := strconv.Atoi(retryAfter)
		if err != nil {
			return nil, fmt.Errorf("invalid Retry-After value")
		}
		con.sugar.Debugf("Rate limit exceeded, pausing for %d seconds\n", retryAfterDuration)
		time.Sleep(time.Duration(retryAfterDuration) * time.Second)
	} else {
		return nil, fmt.Errorf("response from Accrual with StatusCode != StatusOK")
	}

	return accrualResponse, nil
}
