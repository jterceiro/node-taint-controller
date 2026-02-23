package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// annotationUnhealthySince records when a node first became NotReady.
	annotationUnhealthySince = "taint-controller.io/unhealthy-since"

	// outOfServiceTaint is applied to non-gracefully shutdown nodes.
	outOfServiceTaintKey    = "node.kubernetes.io/out-of-service"
	outOfServiceTaintEffect = corev1.TaintEffectNoExecute
)

// NodeReconciler watches Kubernetes nodes and manages the out-of-service taint
// for non-graceful node shutdowns.
type NodeReconciler struct {
	client.Client
	ConfigMapNamespace string
	ConfigMapName      string
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile is the main reconciliation loop for a single node.
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Load configuration from ConfigMap on each reconcile so changes take effect dynamically.
	cfg, err := LoadConfig(ctx, r.Client, r.ConfigMapNamespace, r.ConfigMapName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("loading config: %w", err)
	}

	// Fetch the node.
	node := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Apply node label selector filter.
	if cfg.NodeLabelSelector != "" {
		selector, err := labels.Parse(cfg.NodeLabelSelector)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("parsing node label selector: %w", err)
		}
		if !selector.Matches(labels.Set(node.Labels)) {
			return ctrl.Result{}, nil
		}
	}

	notReady := isNodeNotReady(node)

	if notReady {
		return r.handleNotReadyNode(ctx, node, cfg)
	}
	return r.handleReadyNode(ctx, node, cfg)
}

// handleNotReadyNode processes a node that is currently NotReady.
func (r *NodeReconciler) handleNotReadyNode(ctx context.Context, node *corev1.Node, cfg Config) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", node.Name)
	now := time.Now().UTC()

	// Step 1: Record when the node first became unhealthy.
	unhealthySince, annotated := node.Annotations[annotationUnhealthySince]
	if !annotated {
		patch := client.MergeFrom(node.DeepCopy())
		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}
		node.Annotations[annotationUnhealthySince] = now.Format(time.RFC3339)
		if err := r.Patch(ctx, node, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting unhealthy-since annotation: %w", err)
		}
		logger.Info("Marked node as unhealthy", "since", now)
		unhealthySince = now.Format(time.RFC3339)
	}

	// Step 2: Parse how long the node has been unhealthy.
	firstUnhealthy, err := time.Parse(time.RFC3339, unhealthySince)
	if err != nil {
		// Annotation is malformed; reset it.
		patch := client.MergeFrom(node.DeepCopy())
		node.Annotations[annotationUnhealthySince] = now.Format(time.RFC3339)
		if err := r.Patch(ctx, node, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("resetting unhealthy-since annotation: %w", err)
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	unhealthyDuration := now.Sub(firstUnhealthy)
	delay := time.Duration(cfg.UnhealthyDelayMinutes) * time.Minute

	// Already has the taint; update the gauge and requeue.
	if hasOutOfServiceTaint(node) {
		updateUnhealthyGauge(ctx, r.Client, cfg)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if unhealthyDuration < delay {
		remaining := delay - unhealthyDuration
		logger.Info("Node not yet past unhealthy delay",
			"unhealthyDuration", unhealthyDuration.Round(time.Second),
			"remainingBeforeTaint", remaining.Round(time.Second))
		return ctrl.Result{RequeueAfter: remaining + time.Second}, nil
	}

	// Step 3: Check circuit breaker before applying taint.
	if tripped, err := r.circuitBreakerTripped(ctx, cfg); err != nil {
		return ctrl.Result{}, fmt.Errorf("checking circuit breaker: %w", err)
	} else if tripped {
		logger.Info("Circuit breaker active – skipping taint to prevent cascading failure")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Step 4: Apply the out-of-service taint.
	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Taints = appendTaintIfAbsent(node.Spec.Taints, corev1.Taint{
		Key:    outOfServiceTaintKey,
		Effect: outOfServiceTaintEffect,
	})
	if err := r.Patch(ctx, node, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying out-of-service taint: %w", err)
	}

	nodesTaintedTotal.Inc()
	updateUnhealthyGauge(ctx, r.Client, cfg)
	logger.Info("Applied out-of-service taint", "unhealthyDuration", unhealthyDuration.Round(time.Second))
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// handleReadyNode processes a node that has returned to Ready.
func (r *NodeReconciler) handleReadyNode(ctx context.Context, node *corev1.Node, cfg Config) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", node.Name)

	hasTaint := hasOutOfServiceTaint(node)
	_, hasAnnotation := node.Annotations[annotationUnhealthySince]

	if !hasTaint && !hasAnnotation {
		// Node is healthy and clean; nothing to do.
		return ctrl.Result{}, nil
	}

	// Determine when the node last transitioned to Ready.
	readySince := readyTransitionTime(node)
	recoveryDelay := time.Duration(cfg.RecoveryDelayMinutes) * time.Minute
	if readySince.IsZero() {
		// Ready condition missing (should not happen when node is considered Ready); assume
		// recovery delay has already elapsed so we proceed to remove taint and annotation.
		readySince = time.Now().Add(-(recoveryDelay + time.Second))
	}
	elapsed := time.Since(readySince)

	if elapsed < recoveryDelay {
		remaining := recoveryDelay - elapsed
		logger.Info("Node is Ready but within recovery window",
			"elapsed", elapsed.Round(time.Second),
			"remainingBeforeRemoval", remaining.Round(time.Second))
		return ctrl.Result{RequeueAfter: remaining + time.Second}, nil
	}

	// Recovery period elapsed; remove taint and annotation.
	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Taints = removeTaint(node.Spec.Taints, outOfServiceTaintKey)
	delete(node.Annotations, annotationUnhealthySince)
	if err := r.Patch(ctx, node, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing out-of-service taint: %w", err)
	}

	updateUnhealthyGauge(ctx, r.Client, cfg)
	logger.Info("Removed out-of-service taint after recovery")
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the controller-runtime Manager.
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(r)
}

// isNodeNotReady returns true when the node's Ready condition is False or Unknown.
func isNodeNotReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status != corev1.ConditionTrue
		}
	}
	// No Ready condition → treat as not ready.
	return true
}

// readyTransitionTime returns the last time the node's Ready condition changed to True.
// It returns the zero value time.Time when the Ready condition is missing, so the caller
// must handle that case (e.g. treat as "unknown, assume recovery delay has elapsed").
func readyTransitionTime(node *corev1.Node) time.Time {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			return cond.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

// hasOutOfServiceTaint reports whether the node already carries the out-of-service taint.
func hasOutOfServiceTaint(node *corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == outOfServiceTaintKey && t.Effect == outOfServiceTaintEffect {
			return true
		}
	}
	return false
}

// appendTaintIfAbsent adds taint to the list only if it is not already present.
func appendTaintIfAbsent(taints []corev1.Taint, taint corev1.Taint) []corev1.Taint {
	for _, t := range taints {
		if t.Key == taint.Key && t.Effect == taint.Effect {
			return taints
		}
	}
	return append(taints, taint)
}

// removeTaint returns a copy of the taint slice with the given key removed.
func removeTaint(taints []corev1.Taint, key string) []corev1.Taint {
	result := make([]corev1.Taint, 0, len(taints))
	for _, t := range taints {
		if t.Key != key {
			result = append(result, t)
		}
	}
	return result
}

// countWatchedNodes lists nodes matching cfg.NodeLabelSelector and returns the total
// count and how many of them are NotReady. Used by circuitBreakerTripped and updateUnhealthyGauge.
func countWatchedNodes(ctx context.Context, c client.Client, cfg Config) (total, unhealthy int, err error) {
	nodeList := &corev1.NodeList{}
	listOpts := []client.ListOption{}

	if cfg.NodeLabelSelector != "" {
		selector, parseErr := labels.Parse(cfg.NodeLabelSelector)
		if parseErr != nil {
			return 0, 0, parseErr
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := c.List(ctx, nodeList, listOpts...); err != nil {
		return 0, 0, err
	}

	total = len(nodeList.Items)
	for i := range nodeList.Items {
		if isNodeNotReady(&nodeList.Items[i]) {
			unhealthy++
		}
	}
	return total, unhealthy, nil
}

// circuitBreakerTripped returns true if the percentage of NotReady watched nodes
// exceeds the configured maximum, preventing a cascade of taints.
func (r *NodeReconciler) circuitBreakerTripped(ctx context.Context, cfg Config) (bool, error) {
	total, unhealthy, err := countWatchedNodes(ctx, r.Client, cfg)
	if err != nil {
		return false, err
	}
	if total == 0 {
		return false, nil
	}
	percentage := float64(unhealthy) / float64(total) * 100
	return percentage > cfg.MaxUnhealthyPercentage, nil
}

// updateUnhealthyGauge refreshes the current_unhealthy_nodes Prometheus gauge.
func updateUnhealthyGauge(ctx context.Context, c client.Client, cfg Config) {
	_, unhealthy, err := countWatchedNodes(ctx, c, cfg)
	if err != nil {
		return
	}
	currentUnhealthyNodes.Set(float64(unhealthy))
}
