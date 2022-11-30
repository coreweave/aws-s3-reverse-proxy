package cache

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws/credentials"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/coreweave/aws-s3-reverse-proxy/internal/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"testing"
)

func TestAuthCacheLoad(t *testing.T) {
	// Test Values
	rgwValues := map[string]string{
		"xyz": "abc",
		"abc": "xyz",
	}
	log, _ := zap.NewDevelopment()
	ctrl := gomock.NewController(t)
	mClient := mocks.NewMockAdminClient(ctrl)
	mClient.EXPECT().LoadUserCredentials().Times(1).Return(rgwValues, nil)

	ch := NewAuthCache(mClient, log)

	err := ch.Load()
	assert.NoError(t, err)
}

func TestAuthCacheLoadError(t *testing.T) {
	expectedError := errors.New("error grabbing rgw keys")
	log, _ := zap.NewDevelopment()
	ctrl := gomock.NewController(t)
	mClient := mocks.NewMockAdminClient(ctrl)
	mClient.EXPECT().LoadUserCredentials().Times(1).Return(nil, expectedError)

	ch := NewAuthCache(mClient, log)

	err := ch.Load()
	assert.EqualError(t, expectedError, err.Error())
}

func TestAuthCacheGetCredential(t *testing.T) {
	// Setup Values
	rgwValues := map[string]string{
		"xyz": "abc",
		"abc": "xyz",
	}
	expectedSigners := map[string]*v4.Signer{
		"abc": v4.NewSigner(credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     "abc",
			SecretAccessKey: "xyz",
		})),
	}
	log, _ := zap.NewDevelopment()
	ctrl := gomock.NewController(t)
	mClient := mocks.NewMockAdminClient(ctrl)
	mClient.EXPECT().LoadUserCredentials().Times(1).Return(rgwValues, nil)
	ch := NewAuthCache(mClient, log)
	_ = ch.Load()

	// Test
	output, err := ch.GetRequestSigner("abc")

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedSigners["abc"], output)

	// Test Invalid
	badOutput, realError := ch.GetRequestSigner("bad")

	// Assert Invalid
	assert.EqualError(t, errNoAccessKeyInCache, realError.Error())
	assert.Empty(t, badOutput)
}
