package controller

import (
	"testing"
)

// TestValidate verifies that Config.Validate() catches out-of-range values
// and accepts valid configurations.
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "default config is valid",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
		{
			name: "zero delays are valid",
			cfg: Config{
				UnhealthyDelayMinutes:  0,
				RecoveryDelayMinutes:   0,
				MaxUnhealthyPercentage: 50,
			},
			wantErr: false,
		},
		{
			name: "negative unhealthy delay is invalid",
			cfg: Config{
				UnhealthyDelayMinutes:  -1,
				RecoveryDelayMinutes:   0,
				MaxUnhealthyPercentage: 50,
			},
			wantErr: true,
		},
		{
			name: "negative recovery delay is invalid",
			cfg: Config{
				UnhealthyDelayMinutes:  0,
				RecoveryDelayMinutes:   -1,
				MaxUnhealthyPercentage: 50,
			},
			wantErr: true,
		},
		{
			name: "max-unhealthy-percentage of 0 is invalid",
			cfg: Config{
				UnhealthyDelayMinutes:  0,
				RecoveryDelayMinutes:   0,
				MaxUnhealthyPercentage: 0,
			},
			wantErr: true,
		},
		{
			name: "max-unhealthy-percentage of 1 is valid (lower boundary)",
			cfg: Config{
				UnhealthyDelayMinutes:  0,
				RecoveryDelayMinutes:   0,
				MaxUnhealthyPercentage: 1,
			},
			wantErr: false,
		},
		{
			name: "max-unhealthy-percentage of 100 is valid (upper boundary)",
			cfg: Config{
				UnhealthyDelayMinutes:  0,
				RecoveryDelayMinutes:   0,
				MaxUnhealthyPercentage: 100,
			},
			wantErr: false,
		},
		{
			name: "max-unhealthy-percentage above 100 is invalid",
			cfg: Config{
				UnhealthyDelayMinutes:  0,
				RecoveryDelayMinutes:   0,
				MaxUnhealthyPercentage: 101,
			},
			wantErr: true,
		},
		{
			name: "valid label selector is accepted",
			cfg: Config{
				UnhealthyDelayMinutes:  5,
				RecoveryDelayMinutes:   10,
				MaxUnhealthyPercentage: 33,
				NodeLabelSelector:      "taint-controller=enabled",
			},
			wantErr: false,
		},
		{
			name: "invalid label selector is rejected",
			cfg: Config{
				UnhealthyDelayMinutes:  5,
				RecoveryDelayMinutes:   10,
				MaxUnhealthyPercentage: 33,
				NodeLabelSelector:      "!!invalid-selector",
			},
			wantErr: true,
		},
		{
			name: "empty label selector is valid",
			cfg: Config{
				UnhealthyDelayMinutes:  5,
				RecoveryDelayMinutes:   10,
				MaxUnhealthyPercentage: 33,
				NodeLabelSelector:      "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
