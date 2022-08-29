package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/credentials"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
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
}

func NewAuthCache(rgwAdmin internal.AdminClient, log *zap.Logger, expireTime time.Duration, evictTime time.Duration) *AuthCache {
	ch := cache.New(expireTime, evictTime) // Don't expire
	return &AuthCache{
		rgwAdmin:  rgwAdmin,
		userCache: ch,
		log:       log,
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
	if signer, found := a.userCache.Get(accessKeyId); found {
		return signer.(*v4.Signer), nil
	}
	return nil, errNoAccessKeyInCache
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
