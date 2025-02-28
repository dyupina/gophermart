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
}

func NewController(conf *config.Config, storageService storage.StorageService, logger *zap.SugaredLogger, us user.UserService, wp *WorkerPool) *Controller {
	con := &Controller{
		conf:           conf,
		storageService: storageService,
		sugar:          logger,
		userService:    us,
		workerPool:     wp,
	}
	go con.workerPool.Start(con)
	return con
}

func (con *Controller) Register() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")

		var user user.User
		err := json.NewDecoder(req.Body).Decode(&user)
		login := user.Login
		password := user.Password
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

		con.userService.SetUserIDCookie(res, userID)
		con.Debug(res, "Register success", http.StatusOK)
	}
}

func (con *Controller) Login() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("User-ID")

		var user user.User
		err := json.NewDecoder(req.Body).Decode(&user)
		if err != nil || user.Login == "" || user.Password == "" {
			con.Debug(res, "Bad request", http.StatusBadRequest)
			return
		}

		storedHashedPassword := con.storageService.GetHashedPasswordByLogin(user.Login)
		if storedHashedPassword == "" || !con.storageService.CheckPasswordHash(user.Password, storedHashedPassword) {
			con.Debug(res, "Unauthorized: Invalid login/password", http.StatusUnauthorized)
			return
		}

		// req.Header.Set("User-Login", user.Login)
		err = con.storageService.SaveUID(userID, user.Login)
		if err != nil {
			con.Debug(res, "Bad request", http.StatusBadRequest)
			return
		}
		con.userService.SetUserIDCookie(res, userID)
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
		// orderNumber := string(body)
		orderNumber, _ := strconv.Atoi(string(body))
		if !models.IsValidOrderNumber(strconv.Itoa(orderNumber)) {
			con.Debug(res, "Unprocessable Entity", http.StatusUnprocessableEntity)
			return
		}

		// TEST @@@ Типа совершаем покупку (POST /api/orders)
		utils.MakePurchase(orderNumber, con.conf.AccrualSystemAddress)

		orderAdded, err := con.storageService.AddOrder(userLogin, orderNumber)
		if err != nil {
			if err.Error() == "conflict" {
				con.Debug(res, "Conflict", http.StatusConflict)
				return
			}
			fmt.Printf("\n\n err %s\n\n", err)
			con.Debug(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// // -----------
		// // 3. Заказ попадает в систему расчёта баллов лояльности (в Accrual) @@@
		// _, _ = con.RequestToAccrual(userLogin, orderNumber) // TODO надо проверять ?
		con.workerPool.AddTask(Task{UserLogin: userLogin, OrderNumber: orderNumber})
		// // -----------

		// Ожидание результата выполнения задачи в Worker Pool
		select {
		case result := <-con.workerPool.results:
			fmt.Printf("Order processed: %v\n", result)
		case err := <-con.workerPool.errors:
			fmt.Printf("Error processing order: %v\n", err)
			con.Debug(res, "Error processing order", http.StatusInternalServerError)
			return
		}

		con.userService.SetUserIDCookie(res, userID)
		if orderAdded {
			res.WriteHeader(http.StatusAccepted) // Новый номер заказа принят в обработку
		} else {
			con.Debug(res, "POST orders success", http.StatusOK) // Номер заказа уже был загружен этим пользователем
		}
	}
}

// Запрос в систему расчёта баллов лояльности (в Accrual) @@@ GET /api/orders/{number}
func (con *Controller) RequestToAccrual(userLogin string, orderNumber int) (*models.AccrualResponse, error) {
	fmt.Printf("\n\n@@@@@@ GET /api/orders/{number}\\/\\/\n")

	resp, err := http.Get(fmt.Sprintf("http://%s/api/orders/%d", con.conf.AccrualSystemAddress, orderNumber))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var accrualResponse *models.AccrualResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&accrualResponse); err != nil {
			return nil, err
		}
		status := accrualResponse.Status
		accrual := accrualResponse.Accrual

		fmt.Printf("accrualResponse.Status %s\n", status)
		fmt.Printf("accrualResponse.Accrual %f\n", accrual)

		// Обновить данные о бонусах в таблице users_balances
		if err = con.storageService.UpdateUserBalance(userLogin, orderNumber, accrual); err != nil {
			fmt.Printf(">>> err %s\n", err)
			return nil, fmt.Errorf("error UpdateUserBalance")
		}

		// Обновить данные о заказе в таблице orders
		if err = con.storageService.UpdateOrder(orderNumber, status, accrual); err != nil {
			return nil, fmt.Errorf("error UpdateOrder")
		}

	} else if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		retryAfterDuration, err := strconv.Atoi(retryAfter)
		if err != nil {
			return nil, fmt.Errorf("invalid Retry-After value")
		}
		con.sugar.Debugf("Rate limit exceeded, pausing for %d seconds\n", retryAfterDuration)
		time.Sleep(time.Duration(retryAfterDuration) * time.Second)

	} else {
		return nil, fmt.Errorf("response from Accrual with StatusCode != StatusOK")
	}

	fmt.Printf("/\\/\\@@@@@@ GET /api/orders/{number}\n")
	return accrualResponse, nil
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
			// accrualResponse, errA := con.RequestToAccrual(userLogin, orderNumber)

			tasks[i] = Task{UserLogin: userLogin, OrderNumber: orderNumber}
			con.workerPool.AddTask(tasks[i])
			/*
				// TODO
				if errA != nil {
					if errA.Error() == "error UpdateUserBalance" {
						con.Debug(res, "error UpdateUserBalance", http.StatusInternalServerError) // TODO не знаю какой код отправлять
					} else if errA.Error() == "Error UpdateOrder" {
						con.Debug(res, "Error UpdateOrder", http.StatusInternalServerError) // TODO не знаю какой код отправлять
					}
					// con.Debug(res, "Error communicating with Accrual system", http.StatusInternalServerError)
					return
				} else {
					orders[i].Status = accrualResponse.Status
					orders[i].Accrual = accrualResponse.Accrual
				}*/
		}

		// errorsCount := 0
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
				// errorsCount++
				// fmt.Printf("Error processing order: %v\n", errA)
				if errA.Error() == "error UpdateUserBalance" {
					con.Debug(res, "error UpdateUserBalance", http.StatusInternalServerError) // TODO не знаю какой код отправлять
				} else if errA.Error() == "Error UpdateOrder" {
					con.Debug(res, "Error UpdateOrder", http.StatusInternalServerError) // TODO не знаю какой код отправлять
				}
				return
			}
		}

		// if errorsCount == len(tasks) {
		// 	con.Debug(res, "All requests failed", http.StatusInternalServerError)
		// 	return
		// }

		res.Header().Set("Content-Type", "application/json")
		con.userService.SetUserIDCookie(res, userID)
		res.WriteHeader(http.StatusOK)
		json.NewEncoder(res).Encode(orders)
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
		con.userService.SetUserIDCookie(res, userID)
		res.WriteHeader(http.StatusOK)
		json.NewEncoder(res).Encode(balance)
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
			fmt.Printf(">> RequestForWithdrawal err %s\n", err)
			con.Debug(res, "Unprocessable Entity", http.StatusUnprocessableEntity)
			return
		}

		orderNumber := wr.Order
		if !models.IsValidOrderNumber(orderNumber) {
			fmt.Printf("number %s\n", orderNumber)
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
		con.userService.SetUserIDCookie(res, userID)
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
			fmt.Printf(">>>---- err %s\n", err)
			con.Debug(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if len(withdrawals) == 0 {
			con.Debug(res, "No Withdrawals", http.StatusNoContent)
			return
		}

		res.Header().Set("Content-Type", "application/json")
		con.userService.SetUserIDCookie(res, userID)
		res.WriteHeader(http.StatusOK)
		json.NewEncoder(res).Encode(withdrawals)
	}
}
