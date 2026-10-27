package config

import (
	"context"
	"fmt"
	"mlb-controller/internal/adapters/validator"
	domain "mlb-controller/internal/domain/config"
	certificate_ports "mlb-controller/internal/ports/certificate"
	config_ports "mlb-controller/internal/ports/config"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

type ViperLoader struct {
	path       string
	validator  config_ports.Validator
	certLoader certificate_ports.CertificateLoader
}

func NewViperLoader(path string, certLoader certificate_ports.CertificateLoader) *ViperLoader {
	return &ViperLoader{
		path:       path,
		validator:  validator.NewCompositeValidator(),
		certLoader: certLoader,
	}
}

// StringToDurationHookFunc converts strings to time.Duration for mapstructure decoding
func StringToDurationHookFunc() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t != reflect.TypeOf(time.Duration(0)) {
			return data, nil
		}
		return time.ParseDuration(data.(string))
	}
}

func (l *ViperLoader) Load(ctx context.Context) (*domain.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigFile(l.path)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Register known configuration keys with defaults so v.AllSettings() exposes them when set via environment variables
	v.SetDefault("global_upstream_sync_period", domain.GlobalUpstreamSyncPeriod)
	v.SetDefault("request_pool_size", domain.RequestPoolSize)
	v.SetDefault("allow_empty_upstream", false)
	v.SetDefault("leader_election.enabled", false)
	v.SetDefault("leader_election.lease_duration", domain.LeaderElectionLeaseDuration)
	v.SetDefault("leader_election.renew_deadline", domain.LeaderElectionRenewDeadline)
	v.SetDefault("leader_election.retry_period", domain.LeaderElectionRetryPeriod)
	v.SetDefault("leader_election.resource_lock", "leases")
	v.SetDefault("leader_election.resource_name", "mlb-controller-lease")
	v.SetDefault("leader_election.resource_namespace", "default")
	v.SetDefault("log.level", domain.LogLevel)
	v.SetDefault("log.format", domain.LogFormant)
	v.SetDefault("log.enabled", true)
	v.SetDefault("metrics.port", domain.MetricsPort)
	v.SetDefault("metrics.uri", domain.MetricsURI)
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("kubernetes.resync_period", domain.KubernetesResyncPeriod)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Custom unmarshal for loadbalancers
	var rawConfig struct {
		GlobalUpstreamSyncPeriod time.Duration                  `mapstructure:"global_upstream_sync_period"`
		LeaderElection           domain.LeaderElectionConfig    `mapstructure:"leader_election"`
		Log                      domain.LogConfig               `mapstructure:"log"`
		Metrics                  domain.MetricsConfig           `mapstructure:"metrics"`
		Kubernetes               domain.KubernetesConfig        `mapstructure:"kubernetes"`
		GlobalIPReplacementList  domain.GlobalIPReplacementList `mapstructure:"global_ip_replacement_list"`
		LoadBalancers            []map[string]interface{}       `mapstructure:"loadbalancers"`
		RequestPoolSize          int                            `mapstructure:"request_pool_size"`
		AllowEmptyUpstream       bool                           `mapstructure:"allow_empty_upstream"`
		DefaultBackend           *domain.DefaultBackendConfig   `mapstructure:"default_backend"`
	}

	// Configure mapstructure decoder with DecodeHook
	decoderConfig := &mapstructure.DecoderConfig{
		Result:           &rawConfig,
		DecodeHook:       mapstructure.ComposeDecodeHookFunc(StringToDurationHookFunc()),
		WeaklyTypedInput: true,
	}
	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return nil, fmt.Errorf("create decoder: %w", err)
	}
	if err := decoder.Decode(v.AllSettings()); err != nil {
		return nil, fmt.Errorf("unmarshal raw config: %w", err)
	}

	cfg := &domain.Config{
		GlobalUpstreamSyncPeriod: rawConfig.GlobalUpstreamSyncPeriod,
		LeaderElection:           rawConfig.LeaderElection,
		Log:                      rawConfig.Log,
		Metrics:                  rawConfig.Metrics,
		Kubernetes:               rawConfig.Kubernetes,
		GlobalIPReplacementList:  rawConfig.GlobalIPReplacementList,
		LoadBalancers:            domain.LoadBalancersConfig{},
		RequestPoolSize:          rawConfig.RequestPoolSize,
		AllowEmptyUpstream:       rawConfig.AllowEmptyUpstream,
		DefaultBackend:           rawConfig.DefaultBackend,
	}

	certCtx, certCancel := context.WithTimeout(ctx, 10*time.Second)
	defer certCancel()

	// Unmarshal loadbalancers based on type
	for _, lbRaw := range rawConfig.LoadBalancers {
		typeRaw, ok := lbRaw["type"].(string)
		if !ok {
			return nil, fmt.Errorf("loadbalancer type is missing or invalid")
		}
		typeStr := strings.ToLower(typeRaw)
		switch typeStr {
		case "nginx":
			var nginxCfg domain.NginxConfig
			if err := mapstructureDecode(lbRaw, &nginxCfg); err != nil {
				return nil, fmt.Errorf("unmarshal nginx config: %w", err)
			}

			certContent, err := l.loadCertificatesForLoadBalancer(certCtx, nginxCfg.Name, nginxCfg.CertPath, nginxCfg.KeyPath, nginxCfg.CAPath, nginxCfg.GetHostName())
			if err != nil {
				return nil, err
			}
			nginxCfg.Cert = certContent.Cert
			nginxCfg.Key = certContent.Key
			nginxCfg.CA = certContent.CA

			nginxCfg.Type = typeStr
			cfg.LoadBalancers.LoadBalancers = append(cfg.LoadBalancers.LoadBalancers, nginxCfg)
		case "envoy":
			var envoyCfg domain.EnvoyConfig
			if err := mapstructureDecode(lbRaw, &envoyCfg); err != nil {
				return nil, fmt.Errorf("unmarshal envoy config: %w", err)
			}

			certContent, err := l.loadCertificatesForLoadBalancer(certCtx, envoyCfg.Name, envoyCfg.CertPath, envoyCfg.KeyPath, envoyCfg.CAPath, envoyCfg.GetHostName())
			if err != nil {
				return nil, err
			}
			envoyCfg.Cert = certContent.Cert
			envoyCfg.Key = certContent.Key
			envoyCfg.CA = certContent.CA

			envoyCfg.Type = typeStr
			cfg.LoadBalancers.LoadBalancers = append(cfg.LoadBalancers.LoadBalancers, envoyCfg)
		default:
			return nil, fmt.Errorf("unsupported loadbalancer type: %s", typeStr)
		}
	}

	// Apply default values before validation
	domain.ApplyDefaultValues(cfg)

	if err := l.validator.Validate(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// mapstructureDecode is a helper to decode map to struct using mapstructure
func mapstructureDecode(input interface{}, output interface{}) error {
	config := &mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		Result:           output,
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.ComposeDecodeHookFunc(StringToDurationHookFunc()),
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}
	return decoder.Decode(input)
}

// Helper function to load certificates
func (l *ViperLoader) loadCertificatesForLoadBalancer(ctx context.Context, configName, certPath, keyPath, caPath, hostname string) (*certificate_ports.CertificateContent, error) {
	if certPath == "" && keyPath == "" && caPath == "" {
		return &certificate_ports.CertificateContent{}, nil
	}

	certContent, err := l.certLoader.Load(ctx, certPath, keyPath, caPath, hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificates for %s '%s': %w", strings.ToLower(configName), configName, err)
	}

	return certContent, nil
}
