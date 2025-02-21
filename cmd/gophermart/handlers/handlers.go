package handlers

import (
	"encoding/json"
	"fmt"
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/order"
	"gophermart/cmd/gophermart/storage"
	"gophermart/cmd/gophermart/user"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

type Controller struct {
	conf           *config.Config
	storageService storage.StorageService
	sugar          *zap.SugaredLogger
	userService    user.UserService
}

func NewController(conf *config.Config, storageService storage.StorageService, logger *zap.SugaredLogger, us user.UserService) *Controller {
	return &Controller{
		conf:           conf,
		storageService: storageService,
		sugar:          logger,
		userService:    us,
	}
}

func (con *Controller) Register() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		// userID := req.Header.Get("User-ID")

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
		orderNumber := string(body)
		if !order.IsValidOrderNumber(orderNumber) {
			con.Debug(res, "Unprocessable Entity", http.StatusUnprocessableEntity)
			return
		}

		orderAdded, err := con.storageService.AddOrder(userLogin, orderNumber)
		if err != nil {
			if err.Error() == "conflict" {
				con.Debug(res, "Conflict", http.StatusConflict)
				return
			}

			fmt.Printf("err %v\n", err)
			con.Debug(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if orderAdded {
			res.WriteHeader(http.StatusAccepted) // Новый номер заказа принят в обработку
		} else {
			con.Debug(res, "OK", http.StatusOK) // Номер заказа уже был загружен этим пользователем
		}
	}
}
