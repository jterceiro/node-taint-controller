package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// buildScheme returns a runtime.Scheme with corev1 registered.
func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

// makeNode creates a Node with the specified Ready condition status.
func makeNode(name string, readyStatus corev1.ConditionStatus, annotations map[string]string, taints []corev1.Taint) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
		Spec: corev1.NodeSpec{
			Taints: taints,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             readyStatus,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
				},
			},
		},
	}
}

// makeConfigMap builds a minimal controller ConfigMap.
func makeConfigMap(namespace, name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: data,
	}
}

// TestIsNodeNotReady verifies the helper function for detecting NotReady nodes.
func TestIsNodeNotReady(t *testing.T) {
	tests := []struct {
		name     string
		status   corev1.ConditionStatus
		wantTrue bool
	}{
		{"ready node", corev1.ConditionTrue, false},
		{"not ready – false", corev1.ConditionFalse, true},
		{"not ready – unknown", corev1.ConditionUnknown, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := makeNode("n1", tt.status, nil, nil)
			got := isNodeNotReady(node)
			if got != tt.wantTrue {
				t.Errorf("isNodeNotReady() = %v, want %v", got, tt.wantTrue)
			}
		})
	}
}

// TestDefaultConfig ensures defaults are sane.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.UnhealthyDelayMinutes != defaultUnhealthyDelayMinutes {
		t.Errorf("UnhealthyDelayMinutes = %d, want %d", cfg.UnhealthyDelayMinutes, defaultUnhealthyDelayMinutes)
	}
	if cfg.RecoveryDelayMinutes != defaultRecoveryDelayMinutes {
		t.Errorf("RecoveryDelayMinutes = %d, want %d", cfg.RecoveryDelayMinutes, defaultRecoveryDelayMinutes)
	}
	if cfg.MaxUnhealthyPercentage != defaultMaxUnhealthyPercentage {
		t.Errorf("MaxUnhealthyPercentage = %v, want %v", cfg.MaxUnhealthyPercentage, defaultMaxUnhealthyPercentage)
	}
}

// TestLoadConfig verifies that config values are read from a ConfigMap.
func TestLoadConfig(t *testing.T) {
	cm := makeConfigMap("kube-system", "node-taint-controller-config", map[string]string{
		"unhealthy-delay-minutes":  "2",
		"recovery-delay-minutes":   "3",
		"max-unhealthy-percentage": "50",
		"node-label-selector":      "taint-controller=enabled",
	})

	c := fake.NewClientBuilder().WithScheme(buildScheme()).WithObjects(cm).Build()
	cfg, err := LoadConfig(context.Background(), c, "kube-system", "node-taint-controller-config")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.UnhealthyDelayMinutes != 2 {
		t.Errorf("UnhealthyDelayMinutes = %d, want 2", cfg.UnhealthyDelayMinutes)
	}
	if cfg.RecoveryDelayMinutes != 3 {
		t.Errorf("RecoveryDelayMinutes = %d, want 3", cfg.RecoveryDelayMinutes)
	}
	if cfg.MaxUnhealthyPercentage != 50 {
		t.Errorf("MaxUnhealthyPercentage = %v, want 50", cfg.MaxUnhealthyPercentage)
	}
	if cfg.NodeLabelSelector != "taint-controller=enabled" {
		t.Errorf("NodeLabelSelector = %q, want %q", cfg.NodeLabelSelector, "taint-controller=enabled")
	}
}

// TestLoadConfig_MissingConfigMap verifies that a missing ConfigMap returns defaults.
func TestLoadConfig_MissingConfigMap(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(buildScheme()).Build()
	cfg, err := LoadConfig(context.Background(), c, "kube-system", "missing")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.UnhealthyDelayMinutes != defaultUnhealthyDelayMinutes {
		t.Errorf("expected default, got %d", cfg.UnhealthyDelayMinutes)
	}
}

// TestAppendTaintIfAbsent checks that taints are not duplicated.
func TestAppendTaintIfAbsent(t *testing.T) {
	taint := corev1.Taint{Key: outOfServiceTaintKey, Effect: outOfServiceTaintEffect}
	taints := appendTaintIfAbsent(nil, taint)
	if len(taints) != 1 {
		t.Fatalf("expected 1 taint, got %d", len(taints))
	}
	// Calling again should not add a duplicate.
	taints = appendTaintIfAbsent(taints, taint)
	if len(taints) != 1 {
		t.Fatalf("expected 1 taint after dedup, got %d", len(taints))
	}
}

// TestRemoveTaint verifies taint removal.
func TestRemoveTaint(t *testing.T) {
	taints := []corev1.Taint{
		{Key: "other-key", Effect: corev1.TaintEffectNoSchedule},
		{Key: outOfServiceTaintKey, Effect: outOfServiceTaintEffect},
	}
	result := removeTaint(taints, outOfServiceTaintKey)
	if len(result) != 1 {
		t.Fatalf("expected 1 taint, got %d", len(result))
	}
	if result[0].Key == outOfServiceTaintKey {
		t.Error("out-of-service taint was not removed")
	}
}

// TestReconcile_NotReady_AnnotationSet verifies that the unhealthy-since annotation
// is set when a NotReady node is first reconciled.
func TestReconcile_NotReady_AnnotationSet(t *testing.T) {
	node := makeNode("worker-1", corev1.ConditionFalse, nil, nil)
	cm := makeConfigMap("kube-system", "node-taint-controller-config", map[string]string{
		"unhealthy-delay-minutes": "5",
	})

	c := fake.NewClientBuilder().
		WithScheme(buildScheme()).
		WithObjects(node, cm).
		WithStatusSubresource(node).
		Build()

	r := &NodeReconciler{
		Client:             c,
		ConfigMapNamespace: "kube-system",
		ConfigMapName:      "node-taint-controller-config",
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-1"}})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	updated := &corev1.Node{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "worker-1"}, updated); err != nil {
		t.Fatalf("Get node error: %v", err)
	}

	if _, ok := updated.Annotations[annotationUnhealthySince]; !ok {
		t.Error("expected unhealthy-since annotation to be set")
	}
}

// TestReconcile_NotReady_TaintApplied verifies that the taint is applied once the
// unhealthy delay has been exceeded.
func TestReconcile_NotReady_TaintApplied(t *testing.T) {
	pastTime := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	node := makeNode("worker-2", corev1.ConditionFalse, map[string]string{
		annotationUnhealthySince: pastTime,
	}, nil)
	cm := makeConfigMap("kube-system", "node-taint-controller-config", map[string]string{
		"unhealthy-delay-minutes":  "5",
		"max-unhealthy-percentage": "100", // disable circuit breaker for this test
	})

	c := fake.NewClientBuilder().
		WithScheme(buildScheme()).
		WithObjects(node, cm).
		WithStatusSubresource(node).
		Build()

	r := &NodeReconciler{
		Client:             c,
		ConfigMapNamespace: "kube-system",
		ConfigMapName:      "node-taint-controller-config",
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-2"}})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	updated := &corev1.Node{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "worker-2"}, updated); err != nil {
		t.Fatalf("Get node error: %v", err)
	}

	if !hasOutOfServiceTaint(updated) {
		t.Error("expected out-of-service taint to be applied")
	}
}

// TestReconcile_Ready_TaintRemoved verifies that the taint is removed after recovery delay.
func TestReconcile_Ready_TaintRemoved(t *testing.T) {
	taint := corev1.Taint{Key: outOfServiceTaintKey, Effect: outOfServiceTaintEffect}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-3",
			Annotations: map[string]string{
				annotationUnhealthySince: time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
			},
		},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{taint}},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
					// Node became ready 15 minutes ago; recovery delay is 10 min.
					LastTransitionTime: metav1.NewTime(time.Now().Add(-15 * time.Minute)),
				},
			},
		},
	}
	cm := makeConfigMap("kube-system", "node-taint-controller-config", map[string]string{
		"recovery-delay-minutes": "10",
	})

	c := fake.NewClientBuilder().
		WithScheme(buildScheme()).
		WithObjects(node, cm).
		WithStatusSubresource(node).
		Build()

	r := &NodeReconciler{
		Client:             c,
		ConfigMapNamespace: "kube-system",
		ConfigMapName:      "node-taint-controller-config",
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-3"}})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	updated := &corev1.Node{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "worker-3"}, updated); err != nil {
		t.Fatalf("Get node error: %v", err)
	}

	if hasOutOfServiceTaint(updated) {
		t.Error("expected out-of-service taint to be removed after recovery")
	}
	if _, ok := updated.Annotations[annotationUnhealthySince]; ok {
		t.Error("expected unhealthy-since annotation to be removed")
	}
}

// TestReconcile_CircuitBreaker verifies that no taint is applied when unhealthy
// percentage exceeds the configured maximum.
func TestReconcile_CircuitBreaker(t *testing.T) {
	pastTime := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)

	// Two nodes: both NotReady → 100% unhealthy.
	node1 := makeNode("worker-4", corev1.ConditionFalse, map[string]string{
		annotationUnhealthySince: pastTime,
	}, nil)
	node2 := makeNode("worker-5", corev1.ConditionFalse, nil, nil)

	cm := makeConfigMap("kube-system", "node-taint-controller-config", map[string]string{
		"unhealthy-delay-minutes":  "5",
		"max-unhealthy-percentage": "33", // trip at >33%
	})

	c := fake.NewClientBuilder().
		WithScheme(buildScheme()).
		WithObjects(node1, node2, cm).
		Build()

	r := &NodeReconciler{
		Client:             c,
		ConfigMapNamespace: "kube-system",
		ConfigMapName:      "node-taint-controller-config",
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-4"}})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	updated := &corev1.Node{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "worker-4"}, updated); err != nil {
		t.Fatalf("Get node error: %v", err)
	}

	if hasOutOfServiceTaint(updated) {
		t.Error("circuit breaker should have prevented taint from being applied")
	}
}

// TestReconcile_LabelSelector verifies that nodes not matching the label selector are skipped.
func TestReconcile_LabelSelector(t *testing.T) {
	pastTime := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	node := makeNode("worker-6", corev1.ConditionFalse, map[string]string{
		annotationUnhealthySince: pastTime,
	}, nil)
	// Node does NOT have the required label.

	cm := makeConfigMap("kube-system", "node-taint-controller-config", map[string]string{
		"unhealthy-delay-minutes": "5",
		"node-label-selector":     "taint-controller=enabled",
	})

	c := fake.NewClientBuilder().
		WithScheme(buildScheme()).
		WithObjects(node, cm).
		Build()

	r := &NodeReconciler{
		Client:             c,
		ConfigMapNamespace: "kube-system",
		ConfigMapName:      "node-taint-controller-config",
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker-6"}})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	updated := &corev1.Node{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "worker-6"}, updated); err != nil {
		t.Fatalf("Get node error: %v", err)
	}

	if hasOutOfServiceTaint(updated) {
		t.Error("node without matching label should not be tainted")
	}
}
