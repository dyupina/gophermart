package main

import (
	"errors"
	"gophermart/cmd/gophermart/clients"
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/handlers"
	"gophermart/cmd/gophermart/logger"
	"gophermart/cmd/gophermart/routing"
	"gophermart/cmd/gophermart/storage"
	"gophermart/cmd/gophermart/user"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func main() {
	sugarLogger, err := logger.NewLogger()
	if err != nil {
		sugarLogger.Fatalf("Failed to initialize logger: %v", err)
	}

	c := config.NewConfig()
	err = config.Init(c)
	if err != nil {
		sugarLogger.Fatalf("Failed to initialize config")
	}

	s, err := storage.NewStorage(c)
	if errors.Is(err, storage.ErrOpenDBConnection) {
		sugarLogger.Fatalf("Error opening database connection: %v\n", err)
	} else if errors.Is(err, storage.ErrConnecting) {
		sugarLogger.Fatalf("Error connecting to database: %v\n", err)
	}

	userService := user.NewUserService()
	wp := handlers.NewAccrualQueue(c.NumWorkers, c.MaxRequestsPerMin)
	accrualClient := clients.NewAccrualClient(c.AccrualSystemAddress, sugarLogger)
	ctrl := handlers.NewController(c, s, sugarLogger, userService, wp, accrualClient)

	// Регистрация информации о вознаграждении за товар (POST /api/goods) @@@
	ctrl.AccrualClient.RegisterRewards()

	r := chi.NewRouter()

	routing.InitMiddleware(r, c, ctrl)
	routing.Routing(r, ctrl)

	err = http.ListenAndServe(c.Addr, r) //nolint:gosec // Use chi Timeout (see above)
	if err != nil {
		sugarLogger.Fatalf("Failed to start server: %v", err)
	}
}
