package controller

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultUnhealthyDelayMinutes  = 5
	defaultRecoveryDelayMinutes   = 10
	defaultMaxUnhealthyPercentage = 33.0
	defaultNodeLabelSelector      = ""
	defaultConfigMapNamespace     = "kube-system"
	defaultConfigMapName          = "node-taint-controller-config"
)

// Config holds the controller configuration loaded from a ConfigMap.
type Config struct {
	UnhealthyDelayMinutes  int
	RecoveryDelayMinutes   int
	MaxUnhealthyPercentage float64
	NodeLabelSelector      string
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		UnhealthyDelayMinutes:  defaultUnhealthyDelayMinutes,
		RecoveryDelayMinutes:   defaultRecoveryDelayMinutes,
		MaxUnhealthyPercentage: defaultMaxUnhealthyPercentage,
		NodeLabelSelector:      defaultNodeLabelSelector,
	}
}

// Validate checks that all Config fields satisfy their constraints.
// It returns a non-nil error describing the first violation found.
func (c Config) Validate() error {
	if c.UnhealthyDelayMinutes < 0 {
		return fmt.Errorf("unhealthy-delay-minutes must be >= 0, got %d", c.UnhealthyDelayMinutes)
	}
	if c.RecoveryDelayMinutes < 0 {
		return fmt.Errorf("recovery-delay-minutes must be >= 0, got %d", c.RecoveryDelayMinutes)
	}
	if c.MaxUnhealthyPercentage < 1 || c.MaxUnhealthyPercentage > 100 {
		return fmt.Errorf("max-unhealthy-percentage must be between 1 and 100, got %v", c.MaxUnhealthyPercentage)
	}
	if c.NodeLabelSelector != "" {
		if _, err := labels.Parse(c.NodeLabelSelector); err != nil {
			return fmt.Errorf("invalid node-label-selector %q: %w", c.NodeLabelSelector, err)
		}
	}
	return nil
}

// LoadConfig reads configuration from the controller ConfigMap.
// Missing keys fall back to defaults.
func LoadConfig(ctx context.Context, c client.Client, namespace, name string) (Config, error) {
	cfg := DefaultConfig()

	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cm); err != nil {
		return cfg, client.IgnoreNotFound(err)
	}

	if v, ok := cm.Data["unhealthy-delay-minutes"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.UnhealthyDelayMinutes = n
		}
	}
	if v, ok := cm.Data["recovery-delay-minutes"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.RecoveryDelayMinutes = n
		}
	}
	if v, ok := cm.Data["max-unhealthy-percentage"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 100 {
			cfg.MaxUnhealthyPercentage = f
		}
	}
	if v, ok := cm.Data["node-label-selector"]; ok {
		cfg.NodeLabelSelector = v
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}
