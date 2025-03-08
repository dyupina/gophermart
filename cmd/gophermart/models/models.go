package models

import (
	"time"

	"github.com/EClaesson/go-luhn"
)

type Order struct {
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    float64   `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type UserBalance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type WithdrawRequest struct {
	Order string  `json:"order"` // тут строка (судя по примеру тела запроса POST /api/user/balance/withdraw)
	Sum   float64 `json:"sum"`
}

type Withdrawal struct {
	Order       string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

type AccrualResponse struct {
	Order   string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float64 `json:"accrual,omitempty"`
}

type AccrualGoods struct {
	Description string `json:"description"`
	Price       int    `json:"price"`
}

type AccrualOrder struct {
	Order string         `json:"order"`
	Goods []AccrualGoods `json:"goods"`
}

type RewardRequest struct {
	Match      string `json:"match"`
	Reward     int    `json:"reward"`
	RewardType string `json:"reward_type"`
}

func IsValidOrderNumber(number string) bool {
	res, _ := luhn.IsValid(number)
	return res
}
