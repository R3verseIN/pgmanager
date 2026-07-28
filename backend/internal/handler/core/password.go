package core

import (
	"crypto/rand"
	"log"
	"math/big"
	"regexp"
)

func GeneratePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	password := make([]byte, length)
	for i := range password {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			log.Fatalf("crypto/rand failed: %v", err)
		}
		password[i] = charset[n.Int64()]
	}
	return string(password)
}

var validPasswordPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func ValidPassword(s string) bool {
	if len(s) < 8 || len(s) > 72 {
		return false
	}
	return validPasswordPattern.MatchString(s)
}
