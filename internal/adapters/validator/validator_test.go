package validator

import (
	"mlb-controller/internal/domain/config"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCompositeValidator(t *testing.T) {
	cv := NewCompositeValidator()
	assert.Len(t, cv.validators, 10)
}

func TestCompositeValidator_Validate(t *testing.T) {
	cfg := &config.Config{
		GlobalUpstreamSyncPeriod: 10 * time.Second,
	}
	err := NewCompositeValidator().Validate(cfg)
	assert.NoError(t, err)

	// Test with invalid
	cfg.GlobalUpstreamSyncPeriod = -10 * time.Second
	err = NewCompositeValidator().Validate(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "global_upstream_sync_period must be non-negative")
}

func TestGlobalConfigValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{"Valid", &config.Config{GlobalUpstreamSyncPeriod: 10 * time.Second, RequestPoolSize: 10}, false, ""},
		{"Negative Period", &config.Config{GlobalUpstreamSyncPeriod: -10 * time.Second, RequestPoolSize: 10}, true, "must be non-negative"},
		{"Valid", &config.Config{GlobalUpstreamSyncPeriod: 10 * time.Second, RequestPoolSize: 0}, true, "request_pool_size must be between 1 and 100"},
		{"Valid", &config.Config{GlobalUpstreamSyncPeriod: 10 * time.Second, RequestPoolSize: 101}, true, "request_pool_size must be between 1 and 100"},
		{"Valid", &config.Config{GlobalUpstreamSyncPeriod: 10 * time.Second, RequestPoolSize: 10}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &GlobalConfigValidator{}
			err := v.Validate(tt.config)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
