package user

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

type User struct {
	cookieName string
	cookie     *securecookie.SecureCookie
	Login      string `json:"login"`
	Password   string `json:"password"`
}

type UserService interface {
	GetUserIDFromCookie(r *http.Request) (string, error)
	SetUserIDCookie(res http.ResponseWriter, uid string) error
}

func newSecurecookie() *securecookie.SecureCookie {
	var hashKey = []byte("very-very-very-very-secret-key32")
	var blockKey = []byte("a-lot-of-secret!")
	return securecookie.New(hashKey, blockKey)
}

func NewUserService() *User {
	return &User{
		cookieName: "AuthToken",
		cookie:     newSecurecookie(),
	}
}

func (u *User) GetUserIDFromCookie(req *http.Request) (string, error) {
	cookie, err := req.Cookie(u.cookieName)
	if err != nil {
		return "", err
	}

	var uid string
	if err := u.cookie.Decode(u.cookieName, cookie.Value, &uid); err != nil {
		return "", err
	}

	return uid, nil
}

func (u *User) SetUserIDCookie(res http.ResponseWriter, uid string) error {
	encoded, err := u.cookie.Encode(u.cookieName, uid)

	if err == nil {
		cookie := &http.Cookie{
			Name:    u.cookieName,
			Value:   encoded,
			Path:    "/",
			Secure:  false,
			Expires: time.Now().Add(30 * 24 * time.Hour),
		}
		http.SetCookie(res, cookie)
	} else {
		fmt.Printf("(SetUserIDCookie) err %v\n", err)
	}

	return err
}
