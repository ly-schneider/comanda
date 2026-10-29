package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App          AppConfig
	Postgres     PostgresConfig
	Logger       LoggerConfig
	Security     SecurityConfig
	Payments     PaymentsConfig
	Payrexx      PayrexxConfig
	Plunk        PlunkConfig
	BlobStorage  BlobStorageConfig
	Sentry       SentryConfig
	Android      AndroidConfig
	CustomDomain CustomDomainConfig
	Ingress      IngressConfig
	Inventory    InventoryConfig
	PaymentSync  PaymentReconcileConfig
}

// InventoryConfig drives the pending-order reservation backstop. A checkout
// holds inventory the instant the customer taps pay; an in-flight payment is
// never reclaimed. This backstop only frees stock from genuinely dead pending
// orders (closed tab, no terminal webhook), so its TTL must comfortably exceed
// the real payment window.
type InventoryConfig struct {
	// PendingOrderTTL is how long a pending order may hold its reservation before
	// the backstop sweeper cancels it and releases the stock.
	PendingOrderTTL time.Duration
	// PendingSweepInterval is how often the backstop sweeper runs. Set to 0 to
	// disable the sweeper.
	PendingSweepInterval time.Duration
}

// PaymentReconcileConfig drives the background reconciler that settles orders
// whose success webhook never arrived, so a paid customer isn't left without a
// pickup QR.
type PaymentReconcileConfig struct {
	// Interval is how often a reconcile pass runs. Set to 0 to disable.
	Interval time.Duration
	// MinAge keeps the reconciler off orders that are simply mid-checkout.
	MinAge time.Duration
	// MaxAge bounds the window so an abandoned order is polled a handful of times
	// rather than every pass until the backstop sweeper reclaims it (which
	// verifies once more before releasing).
	MaxAge time.Duration
	// BatchSize caps the provider calls one pass can make.
	BatchSize int
}

// CustomDomainConfig drives bring-your-own custom domains: the DNS records a
// tenant must publish, and the background ownership verifier.
type CustomDomainConfig struct {
	// BaseDomain is the platform's own apex (e.g. comanda.leys.ch). Hostnames at
	// or under it are platform subdomains, not BYO domains, and are rejected.
	BaseDomain        string
	TenantLabelSuffix string
	// CNAMETarget is the value tenants point their custom domain's CNAME at, and
	// the host the ownership check requires the CNAME to resolve to.
	CNAMETarget string
	// OriginHost is a DNS-only hostname resolving straight to the platform's
	// ingress. When set, verification also requires the custom domain to resolve
	// to a subset of its addresses — a matching CNAME alone can still dead-end
	// at a proxy that doesn't know the hostname.
	OriginHost string
	// VerifyInterval is how often the background verifier re-checks DNS for
	// not-yet-verified domains.
	VerifyInterval time.Duration
	// UnverifiedTTL is how long a BYO custom domain may sit unverified before the
	// background sweeper drops it, freeing the globally-unique hostname for the
	// rightful owner to register. Platform-assigned rows carry no verification
	// token and are never swept. Set to 0 to disable the sweeper.
	UnverifiedTTL time.Duration
	// SweepInterval is how often the background sweeper looks for stale unverified
	// domains to drop.
	SweepInterval time.Duration
	// InternalAPIToken gates the cross-tenant trusted-origins feed. When empty
	// the feed is not exposed at all (fail closed).
	InternalAPIToken string
}

// IngressConfig drives the Kubernetes ingress + cert-manager reconciler that
// provisions TLS for verified custom domains.
type IngressConfig struct {
	Enabled      bool
	Namespace    string
	ClassName    string
	HTTP01Issuer string
	// DNS01Issuer is the ClusterIssuer for hostnames at or under ManagedZone, where
	// we control DNS; DNS-01 validates via TXT records and so works even when the name
	// is proxied through Cloudflare (HTTP-01 cannot). Empty disables the DNS-01 path.
	DNS01Issuer string
	// ManagedZone is the apex DNS zone our cert-manager Cloudflare token controls
	// (e.g. comanda.leys.ch). Verified domains at or under it issue via DNS01Issuer;
	// genuine third-party domains use HTTP01Issuer.
	ManagedZone string
	// WildcardBase is the platform subdomain base already served by a *.{WildcardBase}
	// wildcard cert (e.g. staging.comanda.leys.ch). Single-label subdomains of it are
	// covered, so no redundant per-domain certificate is issued.
	WildcardBase   string
	WebService     string
	WebServicePort int
	ReconcileEvery time.Duration
	// ReconcileTimeout bounds a single reconcile pass; a slow/hung cluster call
	// must not stall the ticker indefinitely.
	ReconcileTimeout time.Duration
	Kubeconfig       string
}

type SentryConfig struct {
	DSN              string
	Environment      string
	Release          string
	TracesSampleRate float64
}

type AndroidConfig struct {
	GitHubRepo  string
	GitHubToken string
	// SetupPIN gates Device Setup. Plaintext on purpose: hashing a numeric PIN
	// buys nothing against an offline brute force, so the attempt limiter in
	// front of the endpoint is the actual protection.
	SetupPIN string
	// DownloadSecret derives the per-package APK download token; empty disables it.
	DownloadSecret string
	// RequireDownloadToken must stay false until the whole fleet runs builds that
	// send a token — otherwise those tablets cannot fetch the update that would
	// teach them to send one.
	RequireDownloadToken bool
}

type AppConfig struct {
	AppEnv              string
	AppPort             string
	PublicBaseURL       string
	BootstrapTenantSlug string
	// PlatformHosts are hostnames that resolve to a no-tenant platform context
	// (cross-tenant operation for platform admins) instead of being rejected as
	// unknown. Empty by default.
	PlatformHosts []string
	// TenantHeaderTrusted enables the internal/test X-Tenant-Id resolution path.
	// MUST default false in prod so a forwarded header can never resolve a
	// tenant even if the proxy slips.
	TenantHeaderTrusted bool
}

type PostgresConfig struct {
	DSN             string
	AppDSN          string
	ListenDSN       string
	CAPath          string
	AllowSingleDSN  bool
	MaxConns        int
	MinConns        int
	PrivMaxConns    int
	PrivMinConns    int
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type LoggerConfig struct {
	Level       string
	Development bool
}

type SecurityConfig struct {
	EnableHSTS     bool
	EnableCSP      bool
	TrustedOrigins []string
}

type PaymentsConfig struct {
	MasterKey string // PAYMENTS_MASTER_KEY - base64-encoded 32-byte AES key
}

// PayrexxConfig holds legacy global credentials, used only as an optional
// first-boot seed onto the bootstrap tenant. No-op when empty.
type PayrexxConfig struct {
	InstanceName  string // PAYREXX_INSTANCE - Payrexx instance name
	APISecret     string // PAYREXX_API_SECRET - API secret for HMAC signature
	WebhookSecret string // PAYREXX_WEBHOOK_SECRET - Secret for webhook verification
}

type PlunkConfig struct {
	APIKey    string
	FromName  string
	FromEmail string
	ReplyTo   string
}

type BlobStorageConfig struct {
	Endpoint        string // STORAGE_ENDPOINT - S3 API endpoint (e.g. https://s3.fr-par.scw.cloud)
	AccessKeyID     string // STORAGE_ACCESS_KEY_ID
	SecretAccessKey string // STORAGE_SECRET_ACCESS_KEY
	Bucket          string // STORAGE_BUCKET
	PublicBaseURL   string // STORAGE_PUBLIC_URL - public asset host (CDN domain)
}

func Load() Config {
	// Load .env files by default outside Docker. In Docker, allow opt-in via ALLOW_DOTENV_IN_DOCKER=true
	allowDotenvInDocker := os.Getenv("ALLOW_DOTENV_IN_DOCKER") == "true"
	if !isDockerEnvironment() || allowDotenvInDocker {
		files := []string{".env.local"}

		if appEnv := os.Getenv("APP_ENV"); appEnv != "" && appEnv != "local" {
			envFile := fmt.Sprintf(".env.%s", appEnv)
			if _, err := os.Stat(envFile); err == nil {
				files = append(files, envFile)
			}
		}

		if err := godotenv.Overload(files...); err != nil {
			log.Printf("Warning: could not load env files %v: %v", files, err)
		}
	}

	cfg := Config{
		App: AppConfig{
			AppEnv:              getEnv("APP_ENV"),
			AppPort:             getEnv("APP_PORT"),
			PublicBaseURL:       getEnv("PUBLIC_BASE_URL"),
			BootstrapTenantSlug: getEnvOptional("BOOTSTRAP_TENANT_SLUG"),
			PlatformHosts:       getCommaListOptional("PLATFORM_HOSTS"),
			TenantHeaderTrusted: getEnvBoolOptional("TENANT_HEADER_TRUSTED"),
		},
		Postgres: PostgresConfig{
			DSN:             getEnvOptional("DATABASE_URL"),
			AppDSN:          getEnvOptional("DATABASE_URL_APP"),
			ListenDSN:       getEnvOptional("DATABASE_URL_LISTEN"),
			CAPath:          getEnvOptional("DATABASE_CA_PATH"),
			AllowSingleDSN:  getEnvBoolOptional("ALLOW_SINGLE_DSN"),
			MaxConns:        getEnvIntWithDefault("POSTGRES_MAX_CONNS", 25),
			MinConns:        getEnvIntWithDefault("POSTGRES_MIN_CONNS", 10),
			PrivMaxConns:    getEnvIntWithDefault("POSTGRES_PRIV_MAX_CONNS", 8),
			PrivMinConns:    getEnvIntWithDefault("POSTGRES_PRIV_MIN_CONNS", 4),
			MaxConnLifetime: getEnvDurationWithDefault("POSTGRES_MAX_CONN_LIFETIME", 30*time.Minute),
			MaxConnIdleTime: getEnvDurationWithDefault("POSTGRES_MAX_CONN_IDLE_TIME", 5*time.Minute),
		},
		Logger: LoggerConfig{
			Level:       getEnv("LOG_LEVEL"),
			Development: getEnvAsBool("LOG_DEVELOPMENT"),
		},
		Security: SecurityConfig{
			EnableHSTS:     getEnvAsBool("SECURITY_ENABLE_HSTS"),
			EnableCSP:      getEnvAsBool("SECURITY_ENABLE_CSP"),
			TrustedOrigins: getTrustedOrigins("SECURITY_TRUSTED_ORIGINS"),
		},
		Payments: PaymentsConfig{
			MasterKey: getEnvOptional("PAYMENTS_MASTER_KEY"),
		},
		Payrexx: PayrexxConfig{
			InstanceName:  getEnvOptional("PAYREXX_INSTANCE"),
			APISecret:     getEnvOptional("PAYREXX_API_SECRET"),
			WebhookSecret: getEnvOptional("PAYREXX_WEBHOOK_SECRET"),
		},
		Plunk: PlunkConfig{
			APIKey:    getEnvOptional("PLUNK_API_KEY"),
			FromName:  getEnvOptional("PLUNK_FROM_NAME"),
			FromEmail: getEnvOptional("PLUNK_FROM_EMAIL"),
			ReplyTo:   getEnvOptional("PLUNK_REPLY_TO"),
		},
		BlobStorage: BlobStorageConfig{
			Endpoint:        getEnvOptional("STORAGE_ENDPOINT"),
			AccessKeyID:     getEnvOptional("STORAGE_ACCESS_KEY_ID"),
			SecretAccessKey: getEnvOptional("STORAGE_SECRET_ACCESS_KEY"),
			Bucket:          getEnvWithDefault("STORAGE_BUCKET", "comanda-assets"),
			PublicBaseURL:   getEnvOptional("STORAGE_PUBLIC_URL"),
		},
		Sentry: SentryConfig{
			DSN:              getEnvOptional("SENTRY_DSN"),
			Environment:      getEnvOptional("SENTRY_ENVIRONMENT"),
			Release:          getEnvOptional("APP_VERSION"),
			TracesSampleRate: getEnvFloatWithDefault("SENTRY_TRACES_SAMPLE_RATE", 0.2),
		},
		Android: AndroidConfig{
			GitHubRepo:           getEnvWithDefault("ANDROID_GITHUB_REPO", "ly-schneider/comanda"),
			GitHubToken:          getEnvOptional("ANDROID_GITHUB_TOKEN"),
			SetupPIN:             strings.TrimSpace(getEnvOptional("ANDROID_SETUP_PIN")),
			DownloadSecret:       strings.TrimSpace(getEnvOptional("ANDROID_DOWNLOAD_SECRET")),
			RequireDownloadToken: getEnvBoolOptional("ANDROID_REQUIRE_DOWNLOAD_TOKEN"),
		},
		CustomDomain: CustomDomainConfig{
			BaseDomain:        strings.ToLower(strings.TrimSpace(getEnvOptional("CUSTOM_DOMAIN_BASE"))),
			TenantLabelSuffix: strings.ToLower(strings.TrimSpace(getEnvOptional("TENANT_LABEL_SUFFIX"))),
			CNAMETarget:       strings.ToLower(strings.TrimSpace(getEnvOptional("CUSTOM_DOMAIN_CNAME_TARGET"))),
			OriginHost:        strings.ToLower(strings.TrimSpace(getEnvOptional("CUSTOM_DOMAIN_ORIGIN_HOST"))),
			VerifyInterval:    getEnvDurationWithDefault("CUSTOM_DOMAIN_VERIFY_INTERVAL", 60*time.Second),
			UnverifiedTTL:     getEnvDurationWithDefault("CUSTOM_DOMAIN_UNVERIFIED_TTL", 7*24*time.Hour),
			SweepInterval:     getEnvDurationWithDefault("CUSTOM_DOMAIN_SWEEP_INTERVAL", time.Hour),
			InternalAPIToken:  getEnvOptional("INTERNAL_API_TOKEN"),
		},
		Inventory: InventoryConfig{
			PendingOrderTTL:      getEnvDurationWithDefault("INVENTORY_PENDING_TTL", 30*time.Minute),
			PendingSweepInterval: getEnvDurationWithDefault("INVENTORY_PENDING_SWEEP_INTERVAL", 5*time.Minute),
		},
		PaymentSync: PaymentReconcileConfig{
			Interval:  getEnvDurationWithDefault("PAYMENT_RECONCILE_INTERVAL", 60*time.Second),
			MinAge:    getEnvDurationWithDefault("PAYMENT_RECONCILE_MIN_AGE", 90*time.Second),
			MaxAge:    getEnvDurationWithDefault("PAYMENT_RECONCILE_MAX_AGE", 15*time.Minute),
			BatchSize: getEnvIntWithDefault("PAYMENT_RECONCILE_BATCH_SIZE", 100),
		},
		Ingress: IngressConfig{
			Enabled:          getEnvBoolOptional("INGRESS_RECONCILER_ENABLED"),
			Namespace:        getEnvOptional("INGRESS_NAMESPACE"),
			ClassName:        getEnvWithDefault("INGRESS_CLASS_NAME", "traefik"),
			HTTP01Issuer:     getEnvOptional("INGRESS_HTTP01_ISSUER"),
			DNS01Issuer:      getEnvOptional("INGRESS_DNS01_ISSUER"),
			ManagedZone:      strings.ToLower(strings.TrimSpace(getEnvOptional("INGRESS_MANAGED_ZONE"))),
			WildcardBase:     strings.ToLower(strings.TrimSpace(getEnvOptional("CUSTOM_DOMAIN_BASE"))),
			WebService:       getEnvWithDefault("INGRESS_WEB_SERVICE", "comanda-web"),
			WebServicePort:   getEnvIntWithDefault("INGRESS_WEB_SERVICE_PORT", 80),
			ReconcileEvery:   getEnvDurationWithDefault("INGRESS_RECONCILE_INTERVAL", 60*time.Second),
			ReconcileTimeout: getEnvDurationWithDefault("INGRESS_RECONCILE_TIMEOUT", 30*time.Second),
			Kubeconfig:       getEnvOptional("KUBECONFIG"),
		},
	}

	return cfg
}

// isDockerEnvironment checks if we're running inside a Docker container
func isDockerEnvironment() bool {
	if os.Getenv("DOCKER_CONTAINER") == "true" {
		return true
	}

	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	return false
}

// getEnv gets a environment variable or panics
func getEnv(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	log.Fatalf("config: environment variable %s is not set", key)
	return ""
}

// getEnvAsBool gets an environment variable as a boolean or panics
func getEnvAsBool(key string) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	log.Fatalf("config: environment variable %s is not set", key)
	return false
}

// getEnvBoolOptional gets an environment variable as a boolean, defaulting to
// false when unset or unparseable.
func getEnvBoolOptional(key string) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return false
}

// getTrustedOrigins parses comma-separated list of trusted origins from environment
func getTrustedOrigins(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("config: environment variable %s is not set", key)
		return make([]string, 0)
	}

	origins := strings.Split(value, ",")
	var trimmedOrigins []string
	for _, origin := range origins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			trimmedOrigins = append(trimmedOrigins, trimmed)
		}
	}
	return trimmedOrigins
}

// getEnvOptional gets an environment variable or returns empty string
func getEnvOptional(key string) string {
	return os.Getenv(key)
}

// getCommaListOptional parses a comma-separated env var into a lowercased,
// trimmed slice. Returns nil when unset. Hostnames are compared
// case-insensitively, so they are normalised to lower case here.
func getCommaListOptional(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(strings.ToLower(p)); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// getEnvWithDefault gets an environment variable or returns the default value
func getEnvWithDefault(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// getEnvIntWithDefault parses an integer env var, falling back to def when unset
// or unparseable.
func getEnvIntWithDefault(key string, def int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return def
}

// getEnvFloatWithDefault parses a float env var (e.g. a 0.0–1.0 sample rate),
// falling back to def when unset or unparseable.
func getEnvFloatWithDefault(key string, def float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return def
}

// getEnvDurationWithDefault parses a Go duration (e.g. "60s", "5m") env var,
// falling back to def when unset or unparseable.
func getEnvDurationWithDefault(key string, def time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return def
}
