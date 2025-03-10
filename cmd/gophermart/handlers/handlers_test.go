//go:build unit
// +build unit

package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/logger"
	"gophermart/cmd/gophermart/mocks"
	"gophermart/cmd/gophermart/models"
	"gophermart/cmd/gophermart/storage"
	"gophermart/cmd/gophermart/user"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ShiraazMoollatjie/goluhn"
	"github.com/golang/mock/gomock"
)

func prepare(t *testing.T) (*mocks.MockStorageService, *mocks.MockStorageUtils, *mocks.MockUserService, *mocks.MockAccrualClient, *Controller) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sugarLogger, _ := logger.NewLogger()
	conf := config.NewConfig()
	// _ = config.Init(conf) // TODO ???
	wp := NewAccrualQueue(conf.NumWorkers, conf.MaxRequestsPerMin)
	mockStorageService := mocks.NewMockStorageService(ctrl)
	mockStorageUtils := mocks.NewMockStorageUtils(ctrl)
	mockUserService := mocks.NewMockUserService(ctrl)
	mockAccrualClient := mocks.NewMockAccrualClient(ctrl)

	controller := NewController(conf, mockStorageService, mockStorageUtils, sugarLogger, mockUserService, wp, mockAccrualClient)

	// mockAccrualClient.EXPECT().RegisterRewards().Times(1)
	return mockStorageService, mockStorageUtils, mockUserService, mockAccrualClient, controller
}

func Test_Register(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    user.User
		mockSetup      func(storage *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, userSrv *mocks.MockUserService)
		expectedStatus int
	}{
		{
			name: "Successful Register",
			requestBody: user.User{
				Login:    "testUser",
				Password: "testPassword",
			},
			mockSetup: func(storage *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, userSrv *mocks.MockUserService) {
				// Ожидания для методов, вызываемых в handleAuth
				storage.EXPECT().GetHashedPasswordByLogin("testUser").Return("hashedPassword")
				storageUtils.EXPECT().CheckPasswordHash("testPassword", "hashedPassword").Return(true)
				storage.EXPECT().SaveUID("testUserID", "testUser").Return(nil)

				storageUtils.EXPECT().HashPassword("testPassword").Return("hashedPassword", nil)
				storage.EXPECT().SaveLoginPassword("testUser", "hashedPassword").Return(true)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Conflict",
			requestBody: user.User{
				Login:    "testUserDuplicate",
				Password: "testPasswordDuplicate",
			},
			mockSetup: func(storage *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, userSrv *mocks.MockUserService) {
				storageUtils.EXPECT().HashPassword("testPasswordDuplicate").Return("hashedPasswordDuplicate", nil)
				storage.EXPECT().SaveLoginPassword("testUserDuplicate", "hashedPasswordDuplicate").Return(false)
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name: "Internal server error",
			requestBody: user.User{
				Login:    "testUser",
				Password: "testPassword",
			},
			mockSetup: func(storage *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, userSrv *mocks.MockUserService) {
				storageUtils.EXPECT().HashPassword("testPassword").Return("hashedPassword", errors.New("some err"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, mockStorageUtils, mockUserService, _, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockStorageUtils, mockUserService)

			reqBody, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/api/user/register", bytes.NewReader(reqBody))
			req.Header.Set("User-ID", "testUserID")
			w := httptest.NewRecorder()

			handler := controller.Register()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %v; got %v", tt.expectedStatus, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

func Test_Login(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    user.User
		mockSetup      func(storage *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, userSrv *mocks.MockUserService)
		expectedStatus int
	}{
		{
			name: "Successful Login",
			requestBody: user.User{
				Login:    "testUser",
				Password: "testPassword",
			},
			mockSetup: func(storage *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetHashedPasswordByLogin("testUser").Return("hashedPassword")
				storageUtils.EXPECT().CheckPasswordHash("testPassword", "hashedPassword").Return(true)
				storage.EXPECT().SaveUID("testUserID", "testUser").Return(nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Invalid Password",
			requestBody: user.User{
				Login:    "testUser",
				Password: "wrongPassword",
			},
			mockSetup: func(storage *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, _ *mocks.MockUserService) {
				storage.EXPECT().GetHashedPasswordByLogin("testUser").Return("hashedPassword")
				storageUtils.EXPECT().CheckPasswordHash("wrongPassword", "hashedPassword").Return(false)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "User Not Found",
			requestBody: user.User{
				Login:    "unknownUser",
				Password: "somePassword",
			},
			mockSetup: func(storage *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, _ *mocks.MockUserService) {
				storage.EXPECT().GetHashedPasswordByLogin("unknownUser").Return("")
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Bad Request - Missing Fields",
			requestBody: user.User{
				Login:    "missingPasswordUser",
				Password: "",
			},
			mockSetup: func(_ *mocks.MockStorageService, storageUtils *mocks.MockStorageUtils, _ *mocks.MockUserService) {
			},
			expectedStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, mockStorageUtils, mockUserService, _, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockStorageUtils, mockUserService)

			reqBody, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/api/user/login", bytes.NewReader(reqBody))
			req.Header.Set("User-ID", "testUserID")
			w := httptest.NewRecorder()

			handler := controller.Login()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %v; got %v", tt.expectedStatus, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

func Test_OrdersUpload(t *testing.T) {
	orderNumber := goluhn.Generate(10)
	orderNumberInt, _ := strconv.Atoi(orderNumber)
	errAddOrderConflict := storage.ErrAddOrderConflict
	tests := []struct {
		name           string
		userID         string
		contentType    string
		body           string
		mockSetup      func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient)
		expectedStatus int
	}{
		{
			name:        "Successful Order Upload",
			userID:      "testUserID",
			contentType: "text/plain",
			body:        orderNumber,
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				accSrv.EXPECT().MakePurchase(orderNumber)
				storage.EXPECT().AddOrder("testUser", orderNumberInt).Return(true, nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name:        "Unauthorized User",
			userID:      "unknownUserID",
			contentType: "text/plain",
			body:        orderNumber,
			mockSetup: func(storage *mocks.MockStorageService, _ *mocks.MockUserService, _ *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("unknownUserID").Return("")
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:        "Bad Request - Invalid Content Type",
			userID:      "testUserID",
			contentType: "application/json", // должен быть "text/plain"
			body:        orderNumber,
			mockSetup: func(storage *mocks.MockStorageService, _ *mocks.MockUserService, _ *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Conflict",
			userID:      "testUserID",
			contentType: "text/plain",
			body:        orderNumber,
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				accSrv.EXPECT().MakePurchase(orderNumber)
				storage.EXPECT().AddOrder("testUser", orderNumberInt).Return(true, errAddOrderConflict)
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name:        "Unprocessable Entity",
			userID:      "testUserID",
			contentType: "text/plain",
			body:        "12345678", // неподходящий номер
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				accSrv.EXPECT().MakePurchase("12345678")
				storage.EXPECT().AddOrder("testUser", 12345678).Return(true, errors.New("some err"))
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:        "Internal Server Error",
			userID:      "testUserID",
			contentType: "text/plain",
			body:        orderNumber,
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				accSrv.EXPECT().MakePurchase(orderNumber)
				storage.EXPECT().AddOrder("testUser", orderNumberInt).Return(true, errors.New("not ErrAddOrderConflict"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Duplicate Order Upload",
			userID:      "testUserID",
			contentType: "text/plain",
			body:        orderNumber,
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				accSrv.EXPECT().MakePurchase(orderNumber)
				storage.EXPECT().AddOrder("testUser", orderNumberInt).Return(false, nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, _, mockUserService, mockAccrualClient, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockUserService, mockAccrualClient)

			req := httptest.NewRequest("POST", "/api/user/orders", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("User-ID", tt.userID)
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()

			handler := controller.OrdersUpload()
			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %v; got %v", tt.expectedStatus, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

func Test_OrdersGet(t *testing.T) {
	orderNumber1 := goluhn.Generate(10)
	orderNumber2 := goluhn.Generate(10)

	tests := []struct {
		name           string
		userID         string
		mockSetup      func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient)
		expectedStatus int
		expectedBody   interface{}
	}{
		{
			name:   "Successful Getting Orders",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				accSrv.EXPECT().MakePurchase(orderNumber1)
				storage.EXPECT().GetOrders("testUser").Return([]models.Order{
					{Number: orderNumber2, Status: "PROCESSED", Accrual: 10.0},
					{Number: orderNumber1, Status: "PROCESSING"},
				}, nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: []models.Order{
				{Number: orderNumber2, Status: "PROCESSED", Accrual: 10.0},
				{Number: orderNumber1, Status: "PROCESSING"},
			},
		},
		{
			name:   "Unauthorized User",
			userID: "unknownUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("unknownUserID").Return("")
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   nil,
		},
		{
			name:   "No Content",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetOrders("testUser").Return(nil, nil)
			},
			expectedStatus: http.StatusNoContent,
			expectedBody:   nil,
		},
		{
			name:   "Internal Server Error",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetOrders("testUser").Return(nil, errors.New("some err"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   nil,
		},
		{
			name:   "Internal Server Error (Can't update balance)",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetOrders("testUser").Return(nil, ErrUpdateUserBalance)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   nil,
		},
		{
			name:   "Internal Server Error (Can't update order)",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualClient) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetOrders("testUser").Return(nil, ErrUpdateOrder)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, _, mockUserService, mockAccrualClient, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockUserService, mockAccrualClient)

			handler := controller.OrdersGet()

			req := httptest.NewRequest("GET", "/api/user/orders", nil)
			req.Header.Set("User-ID", tt.userID)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %v; got %v", tt.expectedStatus, resp.StatusCode)
			}
			defer resp.Body.Close()

			if tt.expectedBody != nil {
				var body []models.Order
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}

				expectedBody, _ := json.Marshal(tt.expectedBody)
				actualBody, _ := json.Marshal(body)
				if string(expectedBody) != string(actualBody) {
					t.Errorf("expected body %v; got %v", string(expectedBody), string(actualBody))
				}
			}
		})
	}
}

func Test_UserBalance(t *testing.T) {
	errGetUserBalance := storage.ErrGetUserBalance
	tests := []struct {
		name           string
		userID         string
		mockSetup      func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService)
		expectedStatus int
		expectedBody   interface{}
	}{
		{
			name:   "Successful Getting Balance",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetUserBalance("testUser").Return(models.UserBalance{Current: 100.0, Withdrawn: 20.0}, nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: models.UserBalance{
				Current:   100.0,
				Withdrawn: 20.0,
			},
		},
		{
			name:   "Unauthorized User",
			userID: "unknownUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("unknownUserID").Return("")
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   nil,
		},
		{
			name:   "Internal Server Error",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetUserBalance("testUser").Return(models.UserBalance{}, errGetUserBalance)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, _, mockUserService, _, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockUserService)

			handler := controller.UserBalance()

			req := httptest.NewRequest("GET", "/api/user/balance", nil)
			req.Header.Set("User-ID", tt.userID)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %v; got %v", tt.expectedStatus, resp.StatusCode)
			}
			defer resp.Body.Close()

			if tt.expectedBody != nil {
				var body models.UserBalance
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}

				expectedBody, _ := json.Marshal(tt.expectedBody)
				actualBody, _ := json.Marshal(body)
				if string(expectedBody) != string(actualBody) {
					t.Errorf("expected body %v; got %v", string(expectedBody), string(actualBody))
				}
			}
		})
	}
}

func Test_RequestForWithdrawal(t *testing.T) {
	orderNumber := goluhn.Generate(10)
	orderNumberInt, _ := strconv.Atoi(orderNumber)

	tests := []struct {
		name           string
		userID         string
		requestBody    models.WithdrawRequest
		mockSetup      func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService)
		expectedStatus int
	}{
		{
			name:   "Success Withdrawal Request",
			userID: "testUserID",
			requestBody: models.WithdrawRequest{
				Order: orderNumber,
				Sum:   50.0,
			},
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().WithdrawFromUserBalance("testUser", orderNumberInt, 50.0).Return(nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Unauthorized User",
			userID: "unknownUserID",
			requestBody: models.WithdrawRequest{
				Order: orderNumber,
				Sum:   50.0,
			},
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("unknownUserID").Return("")
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:   "Unprocessable Entity - Invalid Order Number",
			userID: "testUserID",
			requestBody: models.WithdrawRequest{
				Order: "invalidOrder",
				Sum:   50.0,
			},
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:   "Insufficient Funds",
			userID: "testUserID",
			requestBody: models.WithdrawRequest{
				Order: orderNumber,
				Sum:   150.0, // пусть больше доступного баланса
			},
			mockSetup: func(storage_ *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage_.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage_.EXPECT().WithdrawFromUserBalance("testUser", orderNumberInt, 150.0).Return(storage.ErrInsufficientFunds)
			},
			expectedStatus: http.StatusPaymentRequired,
		},
		{
			name:   "Internal Server Error",
			userID: "testUserID",
			requestBody: models.WithdrawRequest{
				Order: orderNumber,
				Sum:   50.0,
			},
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().WithdrawFromUserBalance("testUser", orderNumberInt, 50.0).Return(errors.New("not ErrInsufficientFunds"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, _, mockUserService, _, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockUserService)

			handler := controller.RequestForWithdrawal()

			reqBody, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/api/user/balance/withdraw", bytes.NewReader(reqBody))
			req.Header.Set("User-ID", tt.userID)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %v; got %v", tt.expectedStatus, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

func Test_InfoAboutWithdrawals(t *testing.T) {
	pa1 := time.Now()
	pa2 := time.Now()
	orderNumber1 := goluhn.Generate(10)
	orderNumber2 := goluhn.Generate(10)

	tests := []struct {
		name           string
		userID         string
		mockSetup      func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService)
		expectedStatus int
		expectedBody   interface{}
	}{
		{
			name:   "Success Withdrawals",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetUserWithdrawals("testUser").Return([]models.Withdrawal{
					{Order: orderNumber1, Sum: 50, ProcessedAt: pa1},
					{Order: orderNumber2, Sum: 30, ProcessedAt: pa2},
				}, nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: []models.Withdrawal{
				{Order: orderNumber1, Sum: 50, ProcessedAt: pa1},
				{Order: orderNumber2, Sum: 30, ProcessedAt: pa2},
			},
		},
		{
			name:   "Unauthorized User",
			userID: "unknownUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("unknownUserID").Return("")
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   nil,
		},
		{
			name:   "No Withdrawals",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetUserWithdrawals("testUser").Return(nil, nil)
			},
			expectedStatus: http.StatusNoContent,
			expectedBody:   nil,
		},
		{
			name:   "Internal Server Error",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetUserWithdrawals("testUser").Return([]models.Withdrawal{}, errors.New("some err"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, _, mockUserService, _, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockUserService)

			handler := controller.InfoAboutWithdrawals()

			req := httptest.NewRequest("GET", "/api/user/withdrawals", nil)
			req.Header.Set("User-ID", tt.userID)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %v; got %v", tt.expectedStatus, resp.StatusCode)
			}
			defer resp.Body.Close()

			if tt.expectedBody != nil {
				var body []models.Withdrawal
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}

				expectedBody, _ := json.Marshal(tt.expectedBody)
				actualBody, _ := json.Marshal(body)
				if string(expectedBody) != string(actualBody) {
					t.Errorf("expected body %v; got %v", string(expectedBody), string(actualBody))
				}
			}
		})
	}
}
