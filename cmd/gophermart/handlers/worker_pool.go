package handlers

import (
	"gophermart/cmd/gophermart/models"
	"sync"
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
	wg          *sync.WaitGroup
}

const bufSize = 100

func NewWorkerPool(workerCount, maxRequestsPerMinute int) *WorkerPool {
	interval := time.Minute / time.Duration(maxRequestsPerMinute)
	return &WorkerPool{
		tasks:       make(chan Task, bufSize),
		results:     make(chan *models.AccrualResponse),
		errors:      make(chan error),
		workerCount: workerCount,
		throttle:    time.NewTicker(interval),
		wg:          &sync.WaitGroup{},
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

		wp.wg.Add(1)
		response, err := con.RequestToAccrual(task.UserLogin, task.OrderNumber)
		if err != nil {
			wp.errors <- err
		} else {
			wp.results <- response
		}
		wp.wg.Done()
	}
}

func (wp *WorkerPool) AddTask(task Task) {
	wp.wg.Add(1)
	wp.tasks <- task
}
