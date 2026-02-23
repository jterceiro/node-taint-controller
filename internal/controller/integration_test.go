package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	outOfServiceTaint = "node.kubernetes.io/out-of-service"

	// eventuallyTimeout is how long to wait for the controller to act.
	eventuallyTimeout = 30 * time.Second
	// eventuallyInterval is how frequently to poll.
	eventuallyInterval = 250 * time.Millisecond
)

var _ = Describe("NodeReconciler integration", func() {
	var nodeName string

	BeforeEach(func() {
		// Use a unique node name per test to avoid cross-test interference.
		nodeName = fmt.Sprintf("integration-node-%d", time.Now().UnixNano())
	})

	AfterEach(func() {
		// Clean up the node after each test so the next test starts fresh.
		node := &corev1.Node{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err == nil {
			_ = k8sClient.Delete(ctx, node)
		}
	})

	It("adds the out-of-service taint when Node is NotReady, then removes it when Ready", func() {
		By("creating a Node with no Ready condition (implicitly NotReady)")
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
		}
		Expect(k8sClient.Create(ctx, node)).To(Succeed())

		By("setting the Node status condition to Ready: False")
		statusPatch := client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionFalse,
				Reason:             "KubeletNotReady",
				LastTransitionTime: metav1.NewTime(time.Now()),
			},
		}
		Expect(k8sClient.Status().Patch(ctx, node, statusPatch)).To(Succeed())

		By("asserting that the controller eventually adds the out-of-service taint")
		Eventually(func(g Gomega) {
			updated := &corev1.Node{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, updated)).To(Succeed())
			g.Expect(hasTaint(updated, outOfServiceTaint, corev1.TaintEffectNoExecute)).To(BeTrue(),
				"expected out-of-service taint to be present")
		}, eventuallyTimeout, eventuallyInterval).Should(Succeed())

		By("updating the Node status condition to Ready: True")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)).To(Succeed())
		statusPatch = client.MergeFrom(node.DeepCopy())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionTrue,
				Reason:             "KubeletReady",
				LastTransitionTime: metav1.NewTime(time.Now()),
			},
		}
		Expect(k8sClient.Status().Patch(ctx, node, statusPatch)).To(Succeed())

		By("asserting that the controller eventually removes the out-of-service taint")
		Eventually(func(g Gomega) {
			updated := &corev1.Node{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, updated)).To(Succeed())
			g.Expect(hasTaint(updated, outOfServiceTaint, corev1.TaintEffectNoExecute)).To(BeFalse(),
				"expected out-of-service taint to be removed")
		}, eventuallyTimeout, eventuallyInterval).Should(Succeed())
	})
})

// hasTaint reports whether the node carries a taint with the given key and effect.
func hasTaint(node *corev1.Node, key string, effect corev1.TaintEffect) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == key && t.Effect == effect {
			return true
		}
	}
	return false
}
