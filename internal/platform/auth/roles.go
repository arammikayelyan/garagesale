package auth

import (
	"time"

	jwt "github.com/dgrijalva/jwt-go"
)

// ctxKey represents the type of value for the context key.
type ctxKey int

// Key is used to store/retrieve a Claims value from a context.Context.
const Key ctxKey = 1

// These are the expected values for Claims.Roles
const (
	RoleAdmin = "ADMIN"
	RoleUser  = "USER"
)

// Claims represents the authorization claims transmitted via a JWT
type Claims struct {
	Roles []string `json:"roles"`
	jwt.StandardClaims
}

func NewClaims(subject string, roles []string, now time.Time, expires time.Duration) Claims {
	c := Claims{
		Roles: roles,
		StandardClaims: jwt.StandardClaims{
			Subject:   subject,
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(expires).Unix(),
		},
	}

	return c
}

// HasRole returns true if the claims has at least one of the provided roles.
func (c Claims) HasRole(roles ...string) bool {
	for _, has := range c.Roles {
		for _, want := range roles {
			if has == want {
				return true
			}
		}
	}
	return false
}
