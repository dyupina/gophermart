package handlers

import (
	"encoding/json"
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/storage"
	"gophermart/cmd/gophermart/user"
	"net/http"

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

		ok := con.storageService.SaveLoginPassword(userID, login, hashedPassword)
		if !ok {
			con.Debug(res, "Conflict: Login already taken", http.StatusConflict)
			return
		}

		con.Debug(res, "Register success", http.StatusOK)
	}
}

func (con *Controller) Login() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		// userID := req.Header.Get("User-ID")

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
		con.Debug(res, "Login success", http.StatusOK)
	}
}

func (con *Controller) Debug(res http.ResponseWriter, formatString string, code int) {
	con.sugar.Debugf(formatString)
	if code != http.StatusOK {
		http.Error(res, formatString, code)
	} else {
		res.Write([]byte(formatString + "\n"))
		res.WriteHeader(http.StatusOK)
	}
}
