package cfg

import "gopkg.in/alecthomas/kingpin.v2"

var (
	RgwAdminEndpointEnvVar = "RGW_ENDPOINT"
	RgwAdminSecretEnvVar   = "RGW_SECRET_KEY"
	RgwAdminAccessEnvVar   = "RGW_ACCESS_KEY"
)

// Options for aws-s3-reverse-proxy command line arguments
type Options struct {
	Debug               bool
	ListenAddr          string
	MetricsListenAddr   string
	PprofListenAddr     string
	AllowedSourceSubnet []string
	AwsCredentials      []string
	Region              string
	UpstreamInsecure    bool
	UpstreamEndpoint    string
	CertFile            string
	KeyFile             string
	DisableSSL          bool
	ExpireCacheMinutes  int
	EvictCacheMinutes   int
	RgwAdminEndpoint    string
	RgwAdminAccessKey   string
	RgwAdminSecretKey   string
}

// NewOptions defines and parses the raw command line arguments
func NewOptions() Options {
	var opts Options
	kingpin.Flag("debug", "enable debug logging").Default("false").Envar("DEBUG").BoolVar(&opts.Debug)
	kingpin.Flag("insecure", "enable insecure upstream").Default("false").Envar("INSECURE").BoolVar(&opts.UpstreamInsecure)
	kingpin.Flag("allowed-source-subnet", "allowed source IP addresses with netmask (env - ALLOWED_SOURCE_SUBNET)").Default("127.0.0.1/32").Envar("ALLOWED_SOURCE_SUBNET").StringsVar(&opts.AllowedSourceSubnet)
	kingpin.Flag("upstream-endpoint", "use this S3 endpoint for upstream connections, instead of public AWS S3 (env - UPSTREAM_ENDPOINT)").Envar("UPSTREAM_ENDPOINT").StringVar(&opts.UpstreamEndpoint)
	kingpin.Flag("cert-file", "path to the certificate file (env - CERT_FILE)").Envar("CERT_FILE").Default("").StringVar(&opts.CertFile)
	kingpin.Flag("key-file", "path to the private key file (env - KEY_FILE)").Envar("KEY_FILE").Default("").StringVar(&opts.KeyFile)
	kingpin.Flag("cache-expire", "time in minutes to expire the cache").Default("5").IntVar(&opts.ExpireCacheMinutes)
	kingpin.Flag("cache-evict", "time in minutes to evict the cache").Default("10").IntVar(&opts.EvictCacheMinutes)
	kingpin.Flag("rgw-admin-endpoint", "the rgw admin endpoint to hit").Default("").Default("https://s3.lga1.coreweave.com").Envar(RgwAdminEndpointEnvVar).StringVar(&opts.RgwAdminEndpoint)
	kingpin.Flag("rgw-admin-secret", "the rgw admin secret key").Default("").Envar(RgwAdminSecretEnvVar).StringVar(&opts.RgwAdminSecretKey)
	kingpin.Flag("rgw-admin-access", "the rgw admin access key").Default("").Envar(RgwAdminAccessEnvVar).StringVar(&opts.RgwAdminAccessKey)

	kingpin.Parse()
	return opts
}
