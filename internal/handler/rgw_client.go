package handler

import (
	"context"
	"errors"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreweave/aws-s3-reverse-proxy/internal"
	"net/http"
	"strings"
)

type RgwAdminClient struct {
	client []*admin.API
}

func NewRgwAdminClient(adminAccess, adminSecret, endpoint string) internal.AdminClient {
	endpoints := strings.Split(endpoint, ",")
	keys := strings.Split(adminAccess, ",")
	secrets := strings.Split(adminSecret, ",")
	if len(endpoints) != len(keys) && len(endpoints) != len(secrets) {
		panic(errors.New("mismatched endpoint and key pairs for rgw endpoints"))
	}
	var clients []*admin.API
	for i := 0; i < len(endpoints); i++ {
		goCephClient, err := admin.New(endpoint, adminAccess, adminSecret, http.DefaultClient)
		if err != nil {
			panic(err)
		}
		clients = append(clients, goCephClient)
	}
	return &RgwAdminClient{client: clients}
}

func (r *RgwAdminClient) LoadUserCredentials() (map[string]string, error) {
	ctx := context.Background()
	results := make(map[string]string)
	for _, c := range r.client {
		userResult, err := c.GetUsers(ctx)

		if err != nil {
			return nil, err
		}
		for _, user := range *userResult {
			userInfo, err := c.GetUser(ctx, admin.User{ID: user})
			if err != nil {
				return nil, err
			}
			for _, keys := range userInfo.Keys {
				results[keys.AccessKey] = keys.SecretKey
			}
		}
	}
	return results, nil
}
