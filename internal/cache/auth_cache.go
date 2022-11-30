package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/VictoriaMetrics/fastcache"
	"github.com/aws/aws-sdk-go/aws/credentials"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/coreweave/aws-s3-reverse-proxy/internal"
	"go.uber.org/zap"
	"time"
)

var (
	errNoAccessKeyInCache = errors.New("no accessKeyId found in cache")
)

type AuthCache struct {
	rgwAdmin  internal.AdminClient
	userCache *fastcache.Cache
	log       *zap.Logger
}

func NewAuthCache(rgwAdmin internal.AdminClient, log *zap.Logger) *AuthCache {
	fc := fastcache.New(4000000000) //4GB
	return &AuthCache{
		rgwAdmin:  rgwAdmin,
		userCache: fc,
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
	if secretKey := a.userCache.Get(nil, []byte(accessKeyId)); secretKey != nil {
		return v4.NewSigner(credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: string(secretKey),
		})), nil
	}
	return nil, errNoAccessKeyInCache
}

func (a *AuthCache) Load() (err error) {
	var vals map[string]string
	if vals, err = a.rgwAdmin.LoadUserCredentials(); err == nil {
		a.log.Debug(fmt.Sprintf("loading %d keys from rgw..", len(vals)))
		for k, v := range vals {
			a.userCache.Set([]byte(k), []byte(v))
		}
		return nil
	}
	return err
}
