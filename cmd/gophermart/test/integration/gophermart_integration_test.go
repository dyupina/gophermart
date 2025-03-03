//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gophermart/cmd/gophermart/models"
	"math/rand"
	"net/http"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/ShiraazMoollatjie/goluhn"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
)

type User struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func TestIntegrationGophermart(t *testing.T) {
	accrualUrl := "http://localhost:8085"
	gophermartUrl := "http://localhost:8081"
	rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Not used for cryptographic purposes
	var randInt = 500

	cmd_accrual := exec.Command("../../../accrual/accrual_linux_amd64")
	if err := cmd_accrual.Start(); err != nil {
		t.Fatalf("Failed to start first executable: %v", err)
	}
	defer cmd_accrual.Process.Kill()

	fmt.Printf("accrual started\n")

	cmd_gophermart := exec.Command("../../../../gophermart")
	if err := cmd_gophermart.Start(); err != nil {
		t.Fatalf("Failed to start second executable: %v", err)
	}
	defer cmd_gophermart.Process.Kill()

	fmt.Printf("gophermart started\n")

	time.Sleep(2 * time.Second)

	accrual_client := resty.New()
	gophermart_client := resty.New()

	// 0. Регистрация информации о вознаграждении за товар
	reward1 := models.RewardRequest{Match: "Bork", Reward: 10, RewardType: "%"}
	reward2 := models.RewardRequest{Match: "LG", Reward: 50, RewardType: "pt"}

	accrual_resp, _ := accrual_client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(reward1).
		Post(fmt.Sprintf("%s/api/goods", accrualUrl))
	assert.Equal(t, http.StatusOK, accrual_resp.StatusCode()) // для повторных запусков здесь ошибка - это норм
	accrual_resp, _ = accrual_client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(reward2).
		Post(fmt.Sprintf("%s/api/goods", accrualUrl))
	assert.Equal(t, http.StatusOK, accrual_resp.StatusCode()) // для повторных запусков здесь ошибка - это норм

	// 1. Пользователь регистрируется в системе лояльности «Гофермарт».

	user_n := rand.Intn(randInt) + 1
	u := "user" + strconv.Itoa(user_n)
	user := User{Login: u, Password: u}
	gophermart_resp, _ := gophermart_client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(user).
		Post(fmt.Sprintf("%s/api/user/register", gophermartUrl))
	assert.Equal(t, http.StatusOK, gophermart_resp.StatusCode())
	// Получаем куки из ответа
	cookies := gophermart_resp.Cookies()

	// 2. Пользователь совершает покупку в интернет-магазине «Гофермарт».
	// 3. Заказ попадает в систему расчёта баллов лояльности
	on := goluhn.Generate(10)
	orderJSON := MakePurchase(on)
	fmt.Printf("orderJSON: %s\n", orderJSON)
	accrual_resp, _ = accrual_client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(bytes.NewBuffer(orderJSON)).
		Post(fmt.Sprintf("%s/api/orders", accrualUrl))
	assert.Equal(t, http.StatusAccepted, accrual_resp.StatusCode())

	// 4. Пользователь передаёт номер совершённого заказа в систему лояльности
	gophermart_resp, _ = gophermart_client.R().
		SetHeader("Content-Type", "text/plain").
		SetBody(on).
		SetCookies(cookies).
		Post(fmt.Sprintf("%s/api/user/orders", gophermartUrl))
	assert.Equal(t, http.StatusAccepted, gophermart_resp.StatusCode())

	time.Sleep(2 * time.Second)

	// GET orders
	gophermart_resp, _ = gophermart_client.R().
		SetCookies(cookies).
		Get(fmt.Sprintf("%s/api/user/orders", gophermartUrl))
	assert.Equal(t, http.StatusOK, gophermart_resp.StatusCode())
	fmt.Printf("Orders: %s\n", gophermart_resp.Body())

	// GET balance
	gophermart_resp, _ = gophermart_client.R().
		SetCookies(cookies).
		Get(fmt.Sprintf("%s/api/user/balance", gophermartUrl))
	assert.Equal(t, http.StatusOK, gophermart_resp.StatusCode())

	// POST request for withdrawal
	var user_balance models.UserBalance
	json.Unmarshal(gophermart_resp.Body(), &user_balance)
	var current_balance = user_balance.Current
	var wrJSON = models.WithdrawRequest{Order: on, Sum: current_balance}
	gophermart_resp, _ = gophermart_client.R().
		SetCookies(cookies).
		SetHeader("Content-Type", "application/json").
		SetBody(wrJSON).
		Post(fmt.Sprintf("%s/api/user/balance/withdraw", gophermartUrl))
	assert.Equal(t, http.StatusOK, gophermart_resp.StatusCode())

	// GET balance after withdraw
	gophermart_resp, _ = gophermart_client.R().
		SetCookies(cookies).
		Get(fmt.Sprintf("%s/api/user/balance", gophermartUrl))
	assert.Equal(t, http.StatusOK, gophermart_resp.StatusCode())

	// GET info about withdrawals
	gophermart_resp, _ = gophermart_client.R().
		SetCookies(cookies).
		Get(fmt.Sprintf("%s/api/user/withdrawals", gophermartUrl))
	assert.Equal(t, http.StatusOK, gophermart_resp.StatusCode())

}

func MakePurchase(orderNumber string) []byte {
	n := []string{"Чайник", "Микроволновка", "Холодильник", "Стиральная машина", "Утюг", "Духовой шкаф"}
	b := []string{"Bork", "LG"} //, "Philips", "Samsung"}

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
		Order: orderNumber,
		Goods: goods,
	}

	orderJSON, _ := json.Marshal(order)
	return orderJSON
}
