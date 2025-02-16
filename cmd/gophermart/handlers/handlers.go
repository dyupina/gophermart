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
		if err != nil || user.Login == "" || user.Password == "" {
			http.Error(res, "Bad request", http.StatusBadRequest)
			return
		}

		hashedPassword, err := con.storageService.HashPassword(user.Password)
		if err != nil {
			http.Error(res, "Internal server error", http.StatusInternalServerError)
			return
		}

		ok := con.storageService.SaveLoginPassword(userID, user.Login, hashedPassword)
		if !ok {
			http.Error(res, "Conflict: Login already taken", http.StatusConflict)
			return
		}

		res.WriteHeader(http.StatusOK)
	}
}
