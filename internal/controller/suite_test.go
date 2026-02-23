package controller_test

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/jterceiro/node-taint-controller/internal/controller"
)

// TestControllerSuite is the entry point for the Ginkgo test suite.
func TestControllerSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var (
	cfg            *rest.Config
	k8sClient      client.Client
	testEnv        *envtest.Environment
	envTestStarted bool
	ctx            context.Context
	cancel         context.CancelFunc
)

var _ = BeforeSuite(func() {
	if _, ok := os.LookupEnv("KUBEBUILDER_ASSETS"); !ok {
		Skip("KUBEBUILDER_ASSETS not set – skipping integration tests. " +
			"Run: KUBEBUILDER_ASSETS=$(setup-envtest use 1.31.0 -p path) go test ./...")
	}

	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	testEnv = &envtest.Environment{}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	envTestStarted = true

	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	// Create the kube-system namespace required for the controller ConfigMap.
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	_ = k8sClient.Create(ctx, ns) // ignore error if it already exists

	// Create the controller ConfigMap with 0-minute delays for fast test execution.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "node-taint-controller-config",
		},
		Data: map[string]string{
			"unhealthy-delay-minutes":  "0",
			"recovery-delay-minutes":   "0",
			"max-unhealthy-percentage": "100",
		},
	}
	Expect(k8sClient.Create(ctx, cm)).To(Succeed())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable metrics listener to avoid port conflicts
		},
	})
	Expect(err).NotTo(HaveOccurred())

	Expect((&controller.NodeReconciler{
		Client:             mgr.GetClient(),
		ConfigMapNamespace: "kube-system",
		ConfigMapName:      "node-taint-controller-config",
	}).SetupWithManager(mgr)).To(Succeed())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()
})

var _ = AfterSuite(func() {
	if cancel != nil {
		cancel()
	}
	if envTestStarted {
		Expect(testEnv.Stop()).To(Succeed())
	}
})
