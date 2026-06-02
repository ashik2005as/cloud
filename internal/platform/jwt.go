package platform

import (
	"time"

	"github.com/ashik2005as/cloud/pkg/auth"
)

type JWTClaims = auth.Claims

func ParseJWT(secret, token string) (*JWTClaims, error) {
	return auth.ParseJWT(secret, token)
}

func IssueJWT(secret string, userID int64, email string) (string, error) {
	return auth.IssueJWT(secret, userID, email, 24*time.Hour)
}
