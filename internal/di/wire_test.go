package di

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContainer_Failures(t *testing.T) {
	ctx := context.Background()

	t.Run("Non-existent config file returns error", func(t *testing.T) {
		container, err := NewContainer(ctx, "/path/does/not/exist.yaml")
		assert.Error(t, err)
		assert.Nil(t, container)
	})

	t.Run("Invalid YAML syntax returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgFile := filepath.Join(tmpDir, "invalid.yaml")
		require.NoError(t, os.WriteFile(cfgFile, []byte("loadbalancers: [unclosed"), 0644))

		container, err := NewContainer(ctx, cfgFile)
		assert.Error(t, err)
		assert.Nil(t, container)
	})

	t.Run("Canceled context returns error", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		container, err := NewContainer(canceledCtx, "/nonexistent.yaml")
		assert.Error(t, err)
		assert.Nil(t, container)
	})
}
