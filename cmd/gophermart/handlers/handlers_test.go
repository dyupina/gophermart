package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gophermart/cmd/gophermart/config"
	"gophermart/cmd/gophermart/logger"
	"gophermart/cmd/gophermart/mocks"
	"gophermart/cmd/gophermart/models"
	"gophermart/cmd/gophermart/user"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
)

func prepare(t *testing.T) (*mocks.MockStorageService, *mocks.MockUserService, *mocks.MockAccrualService, *Controller) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sugarLogger, _ := logger.NewLogger()
	conf := config.NewConfig()
	// _ = config.Init(conf) // TODO ???
	wp := NewWorkerPool(conf.NumWorkers, conf.MaxRequestsPerMin)
	mockStorageService := mocks.NewMockStorageService(ctrl)
	mockUserService := mocks.NewMockUserService(ctrl)
	mockAccrualService := mocks.NewMockAccrualService(ctrl)

	controller := NewController(conf, mockStorageService, sugarLogger, mockUserService, wp, mockAccrualService)

	// mockAccrualService.EXPECT().RegisterRewards().Times(1)
	return mockStorageService, mockUserService, mockAccrualService, controller
}

func Test_Register(t *testing.T) {
	mockStorageService, mockUserService, _, controller := prepare(t)

	mockStorageService.EXPECT().HashPassword("testPassword").Return("hashedPassword", nil)
	mockStorageService.EXPECT().SaveLoginPassword("testUser", "hashedPassword").Return(true)
	mockUserService.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)

	reqBody, _ := json.Marshal(map[string]string{
		"login":    "testUser",
		"password": "testPassword",
	})
	req := httptest.NewRequest("POST", "/api/user/register", bytes.NewReader(reqBody))
	req.Header.Set("User-ID", "testUserID")
	w := httptest.NewRecorder()

	handler := controller.Register()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status OK; got %v", resp.StatusCode)
	}
}

func Test_Login(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    user.User
		mockSetup      func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService)
		expectedStatus int
	}{
		{
			name: "Successful Login",
			requestBody: user.User{
				Login:    "testUser",
				Password: "testPassword",
			},
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetHashedPasswordByLogin("testUser").Return("hashedPassword")
				storage.EXPECT().CheckPasswordHash("testPassword", "hashedPassword").Return(true)
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
			mockSetup: func(storage *mocks.MockStorageService, _ *mocks.MockUserService) {
				storage.EXPECT().GetHashedPasswordByLogin("testUser").Return("hashedPassword")
				storage.EXPECT().CheckPasswordHash("wrongPassword", "hashedPassword").Return(false)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "User Not Found",
			requestBody: user.User{
				Login:    "unknownUser",
				Password: "somePassword",
			},
			mockSetup: func(storage *mocks.MockStorageService, _ *mocks.MockUserService) {
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
			mockSetup: func(_ *mocks.MockStorageService, _ *mocks.MockUserService) {
			},
			expectedStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, mockUserService, _, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockUserService)

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
		})
	}
}

func Test_OrdersUpload(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		contentType    string
		body           string
		mockSetup      func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualService)
		expectedStatus int
	}{
		{
			name:        "Successful Order Upload",
			userID:      "testUserID",
			contentType: "text/plain",
			body:        "12345678",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				accSrv.EXPECT().MakePurchase(12345678)
				storage.EXPECT().AddOrder("testUser", 12345678).Return(true, nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name:        "Unauthorized User",
			userID:      "unknownUserID",
			contentType: "text/plain",
			body:        "12345678",
			mockSetup: func(storage *mocks.MockStorageService, _ *mocks.MockUserService, _ *mocks.MockAccrualService) {
				storage.EXPECT().GetLoginByUID("unknownUserID").Return("")
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:        "Bad Request - Invalid Content Type",
			userID:      "testUserID",
			contentType: "application/json", // должен быть "text/plain"
			body:        "12345678",
			mockSetup: func(storage *mocks.MockStorageService, _ *mocks.MockUserService, _ *mocks.MockAccrualService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, mockUserService, mockAccrualService, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockUserService, mockAccrualService)

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
		})
	}
}

func Test_OrdersGet(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		mockSetup      func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualService)
		expectedStatus int
		expectedBody   interface{}
	}{
		{
			name:   "Successful Getting Orders",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				accSrv.EXPECT().MakePurchase(12345678)
				storage.EXPECT().GetOrders("testUser").Return([]models.Order{
					{Number: "23456789", Status: "PROCESSED", Accrual: 10.0},
					{Number: "12345678", Status: "PROCESSING"},
				}, nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: []models.Order{
				{Number: "23456789", Status: "PROCESSED", Accrual: 10.0},
				{Number: "12345678", Status: "PROCESSING"},
			},
		},
		{
			name:   "Unauthorized User",
			userID: "unknownUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualService) {
				storage.EXPECT().GetLoginByUID("unknownUserID").Return("")
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   nil,
		},
		{
			name:   "No Content",
			userID: "testUserID",
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService, accSrv *mocks.MockAccrualService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().GetOrders("testUser").Return(nil, nil)
			},
			expectedStatus: http.StatusNoContent,
			expectedBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, mockUserService, mockAccrualService, controller := prepare(t)
			tt.mockSetup(mockStorageService, mockUserService, mockAccrualService)

			handler := controller.OrdersGet()

			req := httptest.NewRequest("GET", "/api/user/orders", nil)
			req.Header.Set("User-ID", tt.userID)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %v; got %v", tt.expectedStatus, resp.StatusCode)
			}

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, mockUserService, _, controller := prepare(t)
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
				Order: "12345678",
				Sum:   50.0,
			},
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().WithdrawFromUserBalance("testUser", 12345678, 50.0).Return(nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Unauthorized User",
			userID: "unknownUserID",
			requestBody: models.WithdrawRequest{
				Order: "12345678",
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
				Order: "12345678",
				Sum:   150.0, // пусть больше доступного баланса
			},
			mockSetup: func(storage *mocks.MockStorageService, userSrv *mocks.MockUserService) {
				storage.EXPECT().GetLoginByUID("testUserID").Return("testUser")
				storage.EXPECT().WithdrawFromUserBalance("testUser", 12345678, 150.0).Return(fmt.Errorf("insufficient funds"))
			},
			expectedStatus: http.StatusPaymentRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, mockUserService, _, controller := prepare(t)
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
		})
	}
}

func Test_InfoAboutWithdrawals(t *testing.T) {
	pa1 := time.Now()
	pa2 := time.Now()

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
					{Order: "1235678", Sum: 50, ProcessedAt: pa1},
					{Order: "23456789", Sum: 30, ProcessedAt: pa2},
				}, nil)
				userSrv.EXPECT().SetUserIDCookie(gomock.Any(), "testUserID").Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: []models.Withdrawal{
				{Order: "1235678", Sum: 50, ProcessedAt: pa1},
				{Order: "23456789", Sum: 30, ProcessedAt: pa2},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorageService, mockUserService, _, controller := prepare(t)
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
