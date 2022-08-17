module github.com/Kriechi/aws-s3-reverse-proxy

require (
	github.com/aws/aws-sdk-go v1.44.67
	github.com/ceph/go-ceph v0.17.0
	github.com/golang/mock v1.6.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/prometheus/client_golang v1.10.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.8.0
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.22.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
)

go 1.16
