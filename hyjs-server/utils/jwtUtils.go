package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type BClaims struct {
	AdminId   int64
	AdminRole string
	jwt.RegisteredClaims
}

type CClaims struct {
	UserId  int64
	LoginAt int64
	jwt.RegisteredClaims
}

type ResetClaims struct {
	Purpose int64
	UserId  int64
	jwt.RegisteredClaims
}

func GenBToken(jwtKey string, claims BClaims) (string, error) {
	mySigningKey := []byte(jwtKey)
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:   "admin-system",
		IssuedAt: jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(mySigningKey)
}

func GenCToken(jwtKey string, expireHour int64, claims CClaims) (string, error) {
	nw := time.Now()
	mySigningKey := []byte(jwtKey)
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    "user-system",
		ExpiresAt: jwt.NewNumericDate(nw.Add(time.Duration(expireHour) * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(nw),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(mySigningKey)
}

func ParseBToken(jwtKey, tokenString string) (*BClaims, error) {
	mySigningKey := []byte(jwtKey)
	token, err := jwt.ParseWithClaims(tokenString, &BClaims{}, func(token *jwt.Token) (interface{}, error) {
		return mySigningKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*BClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func ParseCToken(jwtKey, tokenString string) (*CClaims, error) {
	mySigningKey := []byte(jwtKey)
	token, err := jwt.ParseWithClaims(tokenString, &CClaims{}, func(token *jwt.Token) (interface{}, error) {
		return mySigningKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*CClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func GenResetToken(jwtKey string, expireMinute int64, claims ResetClaims) (string, error) {
	nw := time.Now()
	mySigningKey := []byte(jwtKey)
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    "user-system",
		ExpiresAt: jwt.NewNumericDate(nw.Add(time.Duration(expireMinute) * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(nw),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(mySigningKey)
}

func ParseResetToken(jwtKey, tokenString string) (*ResetClaims, error) {
	mySigningKey := []byte(jwtKey)
	token, err := jwt.ParseWithClaims(tokenString, &ResetClaims{}, func(token *jwt.Token) (interface{}, error) {
		return mySigningKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*ResetClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}
