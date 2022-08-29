package handler

import (
	"context"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreweave/aws-s3-reverse-proxy/internal"
	"net/http"
)

type RgwAdminClient struct {
	client *admin.API
}

func NewRgwAdminClient(adminAccess, adminSecret, endpoint string) internal.AdminClient {
	goCephClient, err := admin.New(endpoint, adminAccess, adminSecret, http.DefaultClient)
	if err != nil {
		panic(err)
	}
	return &RgwAdminClient{client: goCephClient}
}

func (r *RgwAdminClient) LoadUserCredentials() (map[string]string, error) {
	ctx := context.Background()
	userResult, err := r.client.GetUsers(ctx)

	if err != nil {
		return nil, err
	}
	results := make(map[string]string)
	for _, user := range *userResult {
		userInfo, err := r.client.GetUser(ctx, admin.User{ID: user})
		if err != nil {
			return nil, err
		}
		for _, keys := range userInfo.Keys {
			results[keys.AccessKey] = keys.SecretKey
		}
	}
	return results, nil
}
