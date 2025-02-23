package order

import (
	"unicode"
)

type Order struct {
	Number     string `json:"number"`
	Status     string `json:"status"`
	Accrual    int    `json:"accrual,omitempty"`
	UploadedAt string `json:"uploaded_at"`
}

func IsValidOrderNumber_old(number string) bool {
	for _, n := range number {
		if !unicode.IsDigit(n) {
			return false
		}
	}
	return true
}

// TODO
func IsValidOrderNumber(number int) bool {
	return true
}
