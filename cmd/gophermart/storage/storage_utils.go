package storage

import "golang.org/x/crypto/bcrypt"

type StorageUtils interface {
	HashPassword(password string) (string, error)
	CheckPasswordHash(password, hash string) bool
}

type StorageUtils_ struct {
}

func NewStorageUtils() *StorageUtils_ {
	return &StorageUtils_{}
}

func (su *StorageUtils_) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func (su *StorageUtils_) CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
