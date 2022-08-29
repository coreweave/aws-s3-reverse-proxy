package internal

//go:generate mockgen -destination=../internal/mocks/interfaces.go -package=mocks -source=../internal/interfaces.go

import (
	"context"
	"errors"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"time"
)

var ErrNoAccessKeyFound = errors.New("no access key found in Authorization header")

type AdminClient interface {
	LoadUserCredentials() (map[string]string, error)
}

type AuthParser interface {
	FindAccessKey(authHeader string) (string, error)
}

type AuthCache interface {
	RunSync(interval time.Duration, ctx context.Context)
	GetRequestSigner(accessKeyId string) (*v4.Signer, error)
	Load() (err error)
}
