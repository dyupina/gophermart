package routing

import (
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/handlers"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func InitMiddleware(r *chi.Mux, conf *config.Config, ctrl *handlers.Controller) {
	r.Use(ctrl.PanicRecoveryMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(time.Duration(conf.Timeout) * time.Second))
	r.Use(ctrl.AuthenticateMiddleware)
	r.Use(ctrl.LoggingMiddleware)
	r.Use(ctrl.GzipEncodeMiddleware)
	r.Use(ctrl.GzipDecodeMiddleware)
}

func Routing(r *chi.Mux, ctrl *handlers.Controller) {
	r.Post("/api/user/register", ctrl.Register())
	r.Post("/api/user/login", ctrl.Login())
	r.Post("/api/user/orders", ctrl.OrdersUpload())
	r.Get("/api/user/orders", ctrl.OrdersGet())

}
