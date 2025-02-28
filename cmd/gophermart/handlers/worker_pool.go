package handlers

import (
	"gophermart/cmd/gophermart/models"
	"time"
)

type Task struct {
	UserLogin   string
	OrderNumber int
}

type WorkerPool struct {
	tasks       chan Task
	results     chan *models.AccrualResponse
	errors      chan error
	workerCount int
	throttle    *time.Ticker
}

func NewWorkerPool(workerCount int, maxRequestsPerMinute int) *WorkerPool {
	interval := time.Minute / time.Duration(maxRequestsPerMinute)
	return &WorkerPool{
		tasks:       make(chan Task),
		results:     make(chan *models.AccrualResponse),
		errors:      make(chan error),
		workerCount: workerCount,
		throttle:    time.NewTicker(interval),
	}
}

func (wp *WorkerPool) Start(con *Controller) {
	for i := 0; i < wp.workerCount; i++ {
		go wp.worker(con)
	}
}

func (wp *WorkerPool) worker(con *Controller) {
	for task := range wp.tasks {
		<-wp.throttle.C // Контроль частоты запросов
		response, err := con.RequestToAccrual(task.UserLogin, task.OrderNumber)
		if err != nil {
			wp.errors <- err
		} else {
			wp.results <- response
		}
	}
}

func (wp *WorkerPool) AddTask(task Task) {
	wp.tasks <- task
}
