package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gophermart/cmd/gophermart/models"
	"math/rand"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
)

type AccrualClient interface {
	RequestToAccrualByOrderumber(orderNumber int) (*resty.Response, error)
	MakePurchase(orderNumber int)
	RegisterRewards()
}

type AccrualConf struct {
	AccrualSystemAddress string
	logger               *zap.SugaredLogger
}

func NewAccrualClient(addr string, logger *zap.SugaredLogger) *AccrualConf {
	return &AccrualConf{
		AccrualSystemAddress: addr,
		logger:               logger,
	}
}

func (ac *AccrualConf) MakePurchase(orderNumber int) {
	n := []string{"Чайник", "Микроволновка", "Холодильник", "Стиральная машина", "Утюг", "Духовой шкаф"}
	b := []string{"Bork", "Philips", "Samsung", "LG"}

	rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Not used for cryptographic purposes

	// Случайное количество товаров в заказе
	var randInt = 5
	numberOfGoods := rand.Intn(randInt) + 1 //nolint:gosec // Not used for cryptographic purposes

	var goods []models.AccrualGoods

	for range numberOfGoods {
		// Генерация случайного набора товаров и цены для каждого товара
		desc := fmt.Sprintf("%s %s", n[rand.Intn(len(n))], b[rand.Intn(len(b))]) //nolint:gosec // Not used for cryptographic purposes
		var randPrice = 10000
		price := rand.Intn(randPrice) + 1 //nolint:gosec // Not used for cryptographic purposes

		goods = append(goods, models.AccrualGoods{
			Description: desc,
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
		ac.logger.Errorf("Error marshaling order to JSON:", err)
		return
	}

	// Отправка заказа в систему начисления баллов @@@
	ac.logger.Debugf("(MakePurchase) Order %s\n", bytes.NewBuffer(orderJSON))

	client := resty.New()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(bytes.NewBuffer(orderJSON)).
		Post(fmt.Sprintf("%s/api/orders", ac.AccrualSystemAddress))

	if err != nil {
		ac.logger.Errorf("Error sending POST request:", err)
		return
	}

	ac.logger.Debugf("POST %s/api/orders response status: %s resp.Body %s\n",
		ac.AccrualSystemAddress, resp.Status(), resp.Body())
}

func (ac *AccrualConf) RegisterRewards() {
	brands := []string{"Bork", "Philips", "Samsung", "LG"}
	rewardTypes := []string{"%", "pt"}

	rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Not used for cryptographic purposes

	// Генерация случайных значений для полей запроса
	match := brands[rand.Intn(len(brands))] //nolint:gosec // Not used for cryptographic purposes
	// случайное вознаграждение от 1 до 100
	var randReward = 100
	reward := rand.Intn(randReward) + 1                    //nolint:gosec // Not used for cryptographic purposes
	rewardType := rewardTypes[rand.Intn(len(rewardTypes))] //nolint:gosec // Not used for cryptographic purposes

	rewardRequest := models.RewardRequest{
		Match:      match,
		Reward:     reward,
		RewardType: rewardType,
	}

	rewardJSON, err := json.Marshal(rewardRequest)
	if err != nil {
		ac.logger.Errorf("Error marshaling rewardRequest to JSON:", err)
		return
	}

	client := resty.New()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(bytes.NewBuffer(rewardJSON)).
		Post(fmt.Sprintf("%s/api/goods", ac.AccrualSystemAddress))

	if err != nil {
		ac.logger.Errorf("Error sending POST request:", err)
		return
	}

	ac.logger.Debugf("POST %s/api/goods response status: %s\n", ac.AccrualSystemAddress, resp.Status())
}

func (ac *AccrualConf) RequestToAccrualByOrderumber(orderNumber int) (*resty.Response, error) {
	client := resty.New()
	resp, err := client.R().
		Get(fmt.Sprintf("%s/api/orders/%d", ac.AccrualSystemAddress, orderNumber))

	if err != nil {
		return nil, err
	}

	return resp, nil
}
