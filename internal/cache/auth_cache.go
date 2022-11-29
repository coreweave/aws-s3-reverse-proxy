package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/credentials"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/cenkalti/backoff"
	"github.com/coreweave/aws-s3-reverse-proxy/internal"
	"github.com/patrickmn/go-cache"
	"go.uber.org/zap"
	"time"
)

var (
	errNoAccessKeyInCache = errors.New("no accessKeyId found in cache")
)

type AuthCache struct {
	rgwAdmin  internal.AdminClient
	userCache *cache.Cache
	log       *zap.Logger
	boff      backoff.BackOff
}

func NewAuthCache(rgwAdmin internal.AdminClient, log *zap.Logger, expireTime time.Duration, evictTime time.Duration) *AuthCache {
	ch := cache.New(expireTime, evictTime) // Don't expire
	boff := backoff.NewExponentialBackOff()
	boff.MaxElapsedTime = time.Duration(1) * time.Second
	boff.MaxInterval = time.Duration(250) * time.Millisecond
	boff.InitialInterval = time.Duration(5) * time.Millisecond
	return &AuthCache{
		rgwAdmin:  rgwAdmin,
		userCache: ch,
		log:       log,
		boff:      boff,
	}
}

func (a *AuthCache) RunSync(interval time.Duration, ctx context.Context) {
	go func(a *AuthCache, ctx context.Context) {
		t := time.Tick(interval)
		for {
			select {
			case <-t:
				a.log.Sugar().Infof("Starting rgw cache sync at %d", time.Now().UnixMilli())
				if err := a.Load(); err != nil {
					a.log.Error("unable to load user creds")
				}
				a.log.Sugar().Infof("finished rgw cache sync at %d", time.Now().UnixMilli())
			case <-ctx.Done():
				return
			}
		}
	}(a, ctx)
}

func (a *AuthCache) GetRequestSigner(accessKeyId string) (*v4.Signer, error) {
	var foundSigner *v4.Signer
	// Under Load this call to the cache returns false for accessKeys randomly. Simple retry logic fixes.
	retryFn := func() error {
		if signer, found := a.userCache.Get(accessKeyId); found {
			foundSigner = signer.(*v4.Signer)
			return nil
		}
		return errNoAccessKeyInCache
	}

	if err := backoff.Retry(retryFn, a.boff); err != nil {
		return nil, err
	}
	return foundSigner, nil
}

func (a *AuthCache) Load() (err error) {
	var vals map[string]string
	if vals, err = a.rgwAdmin.LoadUserCredentials(); err == nil {
		a.log.Debug(fmt.Sprintf("loading %d keys from rgw..", len(vals)))
		for k, v := range vals {
			signer := v4.NewSigner(credentials.NewStaticCredentialsFromCreds(credentials.Value{
				AccessKeyID:     k,
				SecretAccessKey: v,
			}))
			a.userCache.Set(k, signer, 0)
		}
		return nil
	}
	return err
}
