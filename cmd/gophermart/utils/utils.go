package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gophermart/cmd/gophermart/models"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

type AccrualService interface {
	RequestToAccrualByOrderumber(orderNumber int) (*http.Response, error)
	MakePurchase(orderNumber int)
	RegisterRewards()
}

type AccrualConf struct {
	AccrualSystemAddress string
}

func NewAccrualService(addr string) AccrualService {
	return &AccrualConf{
		AccrualSystemAddress: addr,
	}
}

func (ac *AccrualConf) MakePurchase(orderNumber int) {
	names := []string{"Чайник", "Микроволновка", "Холодильник", "Стиральная машина", "Утюг", "Духовой шкаф"}
	brands := []string{"Bork", "Philips", "Samsung", "LG"}

	rand.New(rand.NewSource(time.Now().UnixNano()))

	// Случайное количество товаров в заказе
	numberOfGoods := rand.Intn(5) + 1 // случайное количество от 1 до 5

	var goods []models.AccrualGoods

	for range numberOfGoods {
		// Генерация случайного набора товаров и цены для каждого товара
		description := fmt.Sprintf("%s %s", names[rand.Intn(len(names))], brands[rand.Intn(len(brands))])
		price := rand.Intn(10000) + 1

		goods = append(goods, models.AccrualGoods{
			Description: description,
			Price:       price,
		})
	}

	// Формирование заказа
	order := models.AccrualOrder{
		Order: strconv.Itoa(orderNumber),
		Goods: goods,
	}

	orderJSON, err := json.Marshal(order)
	if err != nil {
		fmt.Println("Error marshaling order to JSON:", err)
		return
	}

	// Отправка заказа в систему начисления баллов @@@
	fmt.Printf("(MakePurchase) Order %s\n", bytes.NewBuffer(orderJSON))
	resp, err := http.Post(fmt.Sprintf("http://%s/api/orders", ac.AccrualSystemAddress), "application/json", bytes.NewBuffer(orderJSON))
	if err != nil {
		fmt.Println("Error sending POST request:", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	fmt.Printf("POST http://%s/api/orders response status: %s resp.Body %s\n", ac.AccrualSystemAddress, resp.Status, bodyBytes)

}

func (ac *AccrualConf) RegisterRewards() {
	brands := []string{"Bork", "Philips", "Samsung", "LG"}
	rewardTypes := []string{"%", "pt"}

	rand.New(rand.NewSource(time.Now().UnixNano()))

	// Генерация случайных значений для полей запроса
	match := brands[rand.Intn(len(brands))]
	reward := rand.Intn(100) + 1 // случайное вознаграждение от 1 до 100
	rewardType := rewardTypes[rand.Intn(len(rewardTypes))]

	rewardRequest := models.RewardRequest{
		Match:      match,
		Reward:     reward,
		RewardType: rewardType,
	}

	rewardJSON, err := json.Marshal(rewardRequest)
	if err != nil {
		fmt.Println("Error marshaling rewardRequest to JSON:", err)
		return
	}

	resp, err := http.Post(fmt.Sprintf("http://%s/api/goods", ac.AccrualSystemAddress), "application/json", bytes.NewBuffer(rewardJSON))
	if err != nil {
		fmt.Println("Error sending POST request:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("POST http://%s/api/goods response status: %s\n", ac.AccrualSystemAddress, resp.Status)
}

func (ac *AccrualConf) RequestToAccrualByOrderumber(orderNumber int) (*http.Response, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/api/orders/%d", ac.AccrualSystemAddress, orderNumber))
	if err != nil {
		return nil, err
	}

	return resp, nil
}
