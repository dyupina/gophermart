package order

import (
	"time"
	"unicode"
)

type Order struct {
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    int       `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

func IsValidOrderNumber(number string) bool {
	for _, n := range number {
		if !unicode.IsDigit(n) {
			return false
		}
	}
	return true
}
