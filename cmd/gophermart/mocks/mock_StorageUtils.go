// Code generated by MockGen. DO NOT EDIT.
// Source: gophermart/cmd/gophermart/storage (interfaces: StorageUtils)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockStorageUtils is a mock of StorageUtils interface.
type MockStorageUtils struct {
	ctrl     *gomock.Controller
	recorder *MockStorageUtilsMockRecorder
}

// MockStorageUtilsMockRecorder is the mock recorder for MockStorageUtils.
type MockStorageUtilsMockRecorder struct {
	mock *MockStorageUtils
}

// NewMockStorageUtils creates a new mock instance.
func NewMockStorageUtils(ctrl *gomock.Controller) *MockStorageUtils {
	mock := &MockStorageUtils{ctrl: ctrl}
	mock.recorder = &MockStorageUtilsMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStorageUtils) EXPECT() *MockStorageUtilsMockRecorder {
	return m.recorder
}

// CheckPasswordHash mocks base method.
func (m *MockStorageUtils) CheckPasswordHash(arg0, arg1 string) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CheckPasswordHash", arg0, arg1)
	ret0, _ := ret[0].(bool)
	return ret0
}

// CheckPasswordHash indicates an expected call of CheckPasswordHash.
func (mr *MockStorageUtilsMockRecorder) CheckPasswordHash(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CheckPasswordHash", reflect.TypeOf((*MockStorageUtils)(nil).CheckPasswordHash), arg0, arg1)
}

// HashPassword mocks base method.
func (m *MockStorageUtils) HashPassword(arg0 string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HashPassword", arg0)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HashPassword indicates an expected call of HashPassword.
func (mr *MockStorageUtilsMockRecorder) HashPassword(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HashPassword", reflect.TypeOf((*MockStorageUtils)(nil).HashPassword), arg0)
}
