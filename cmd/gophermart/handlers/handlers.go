package handlers

import (
	"encoding/json"
	"errors"
	"gophermart/cmd/gophermart/clients"
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/models"
	"gophermart/cmd/gophermart/storage"
	"gophermart/cmd/gophermart/user"
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
	storageUtils   storage.StorageUtils
	sugar          *zap.SugaredLogger
	userService    user.UserService
	accrualQueue   *AccrualQueue
	AccrualClient  clients.AccrualClient
}

var (
	ErrUpdateUserBalance = errors.New("error UpdateUserBalance")
	ErrUpdateOrder       = errors.New("error UpdateOrder")
	ErrRetryAfter        = errors.New("error invalid Retry-After value")
	ErrNoOKfromAccrual   = errors.New("response from Accrual with StatusCode != StatusOK")
	ErrAccrualRequest    = errors.New("error sending GET request")
)

func NewController(conf *config.Config, storageService storage.StorageService, storageUtils storage.StorageUtils,
	logger *zap.SugaredLogger, us user.UserService, wp *AccrualQueue, accrualService clients.AccrualClient) *Controller {
	con := &Controller{
		conf:           conf,
		storageService: storageService,
		storageUtils:   storageUtils,
		sugar:          logger,
		userService:    us,
		accrualQueue:   wp,
		AccrualClient:  accrualService,
	}

	con.accrualQueue.Start(con)

	return con
}

func (con *Controller) handleAuth(res http.ResponseWriter, userID string, user_ user.User) {
	storedHashedPassword := con.storageService.GetHashedPasswordByLogin(user_.Login)
	if storedHashedPassword == "" || !con.storageUtils.CheckPasswordHash(user_.Password, storedHashedPassword) {
		con.Debug(res, "Unauthorized: Invalid login/password", http.StatusUnauthorized)
		return
	}

	err := con.storageService.SaveUID(userID, user_.Login)
	if err != nil {
		con.Debug(res, "Bad request", http.StatusBadRequest)
		return
	}

	_ = con.userService.SetUserIDCookie(res, userID)
	con.Debug(res, "Login success", http.StatusOK)
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

		hashedPassword, err := con.storageUtils.HashPassword(password)
		if err != nil {
			con.Debug(res, "(Register) Internal server error", http.StatusInternalServerError)
			return
		}

		ok := con.storageService.SaveLoginPassword(login, hashedPassword)
		if !ok {
			con.Debug(res, "Conflict: Login already taken", http.StatusConflict)
			return
		}

		con.handleAuth(res, userID, user_)
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
		con.handleAuth(res, userID, user_)
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
		// con.AccrualClient.MakePurchase(orderNumber)

		orderAdded, err := con.storageService.AddOrder(userLogin, orderNumber)
		if err != nil {
			if errors.Is(err, storage.ErrAddOrderConflict) {
				con.Debug(res, "Conflict", http.StatusConflict)
				return
			}
			con.Debug(res, "(OrdersUpload) Internal Server Error", http.StatusInternalServerError)
			return
		}

		// 3. Заказ попадает в систему расчёта баллов лояльности (в Accrual) @@@
		con.accrualQueue.AddTask(Task{UserLogin: userLogin, OrderNumber: orderNumber})

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
			con.Debug(res, "(OrdersGet) Unauthorized", http.StatusUnauthorized)
			return
		}

		orders, err := con.storageService.GetOrders(userLogin)
		if err != nil {
			con.Debug(res, "(OrdersGet) Internal Server Error", http.StatusInternalServerError)
			return
		}

		if len(orders) == 0 {
			con.Debug(res, "(OrdersGet) No Content", http.StatusNoContent)
			return
		}

		tasks := make([]Task, len(orders))
		// Обновление статусов заказов через систему расчёта начислений (Accrual) @@@
		for i, order := range orders {
			orderNumber, _ := strconv.Atoi(order.Number)
			tasks[i] = Task{UserLogin: userLogin, OrderNumber: orderNumber}
			con.accrualQueue.AddTask(tasks[i])
		}

		go func() {
			for range tasks {
				select {
				case result := <-con.accrualQueue.results:
					for i, order := range orders {
						if order.Number == result.Order {
							orders[i].Status = result.Status
							orders[i].Accrual = result.Accrual
							break
						}
					}
				case errA := <-con.accrualQueue.errors:
					switch {
					case errors.Is(errA, ErrUpdateUserBalance):
						con.Debug(res, "(OrdersGet) Error UpdateUserBalance", http.StatusInternalServerError)
					case errors.Is(errA, ErrUpdateOrder):
						con.Debug(res, "(OrdersGet) Error UpdateOrder", http.StatusInternalServerError)
						// default:
						// 	con.Debug(res, "(OrdersGet) Internal Server Error", http.StatusInternalServerError)
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
			con.Debug(res, "(UserBalance) Internal Server Error", http.StatusInternalServerError)
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
			if errors.Is(err, storage.ErrInsufficientFunds) {
				con.Debug(res, "Insufficient funds", http.StatusPaymentRequired)
			} else {
				con.Debug(res, "(RequestForWithdrawal) Internal Server Error", http.StatusInternalServerError)
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
			con.Debug(res, "(InfoAboutWithdrawals) Internal Server Error", http.StatusInternalServerError)
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
	resp, err := con.AccrualClient.RequestToAccrualByOrderumber(orderNumber)
	if err != nil {
		return nil, ErrAccrualRequest
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
			return nil, ErrUpdateUserBalance
		}

		// Обновить данные о заказе в таблице orders
		if err = con.storageService.UpdateOrder(orderNumber, status, accrual); err != nil {
			return nil, ErrUpdateOrder
		}
	} else if resp.StatusCode() == http.StatusTooManyRequests {
		retryAfter := resp.Header().Get("Retry-After")
		retryAfterDuration, err := strconv.Atoi(retryAfter)
		if err != nil {
			return nil, ErrRetryAfter
		}
		con.sugar.Debugf("Rate limit exceeded, pausing for %d seconds\n", retryAfterDuration)
		time.Sleep(time.Duration(retryAfterDuration) * time.Second)
	} else {
		return nil, ErrNoOKfromAccrual
	}

	return accrualResponse, nil
}
