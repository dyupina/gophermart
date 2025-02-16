package main

import (
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/handlers"
	"gophermart/cmd/gophermart/logger"
	"gophermart/cmd/gophermart/routing"
	db "gophermart/cmd/gophermart/storage"
	"gophermart/cmd/gophermart/user"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func main() {
	c := config.NewConfig()
	config.Init(c)

	s := db.NewStorage(c)

	sugarLogger, err := logger.NewLogger()
	if err != nil {
		sugarLogger.Fatalf("Failed to initialize logger: %v", err)
	}

	userService := user.NewUserService()
	ctrl := handlers.NewController(c, s, sugarLogger, userService)

	r := chi.NewRouter()

	routing.InitMiddleware(r, c, ctrl)
	routing.Routing(r, ctrl)

	err = http.ListenAndServe(c.Addr, r) //nolint:gosec // Use chi Timeout (see above)
	if err != nil {
		sugarLogger.Fatalf("Failed to start server: %v", err)
	}
}
