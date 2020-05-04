package manager_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/joelanford/helm-operator/pkg/manager"
)

var _ = Describe("ConfigureWatchNamespaces", func() {
	var (
		opts manager.Options
		log  logr.Logger = &testing.NullLogger{}
	)

	BeforeEach(func() {
		opts = manager.Options{}
		Expect(os.Unsetenv(WatchNamespaceEnvVar)).To(Succeed())
	})

	It("should watch all namespaces when no env set", func() {
		ConfigureWatchNamespaces(&opts, log)
		Expect(opts.Namespace).To(Equal(""))
		Expect(opts.NewCache).To(BeNil())
	})

	It("should watch all namespaces when WATCH_NAMESPACE is empty", func() {
		Expect(os.Setenv(WatchNamespaceEnvVar, ""))
		ConfigureWatchNamespaces(&opts, log)
		Expect(opts.Namespace).To(Equal(""))
		Expect(opts.NewCache).To(BeNil())
	})

	It("should watch one namespace when WATCH_NAMESPACE is has one namespace", func() {
		Expect(os.Setenv(WatchNamespaceEnvVar, "watch"))
		ConfigureWatchNamespaces(&opts, log)
		Expect(opts.Namespace).To(Equal("watch"))
		Expect(opts.NewCache).To(BeNil())
	})

	It("should watch multiple namespaces when WATCH_NAMESPACE has multiple namespaces", func() {
		By("creating pods in watched namespaces")
		watchedPods, err := createPods(context.TODO(), 2)
		Expect(err).To(BeNil())

		By("creating pods in watched namespaces")
		unwatchedPods, err := createPods(context.TODO(), 2)
		Expect(err).To(BeNil())

		By("configuring WATCH_NAMESPACE with the namespaces of the watched pods")
		Expect(os.Setenv(WatchNamespaceEnvVar, strings.Join(getNamespaces(watchedPods), ",")))
		ConfigureWatchNamespaces(&opts, log)

		By("checking that a single-namespace watch is not configured")
		Expect(opts.Namespace).To(Equal(""))

		By("using the options NewCache function to create a cache")
		c, err := opts.NewCache(cfg, cache.Options{})
		Expect(err).To(BeNil())

		By("starting the cache and waiting for it to sync")
		done := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			Expect(c.Start(done)).To(Succeed())
			wg.Done()
		}()
		Expect(c.WaitForCacheSync(done)).To(BeTrue())

		By("successfully getting the watched pods")
		for _, p := range watchedPods {
			key, err := client.ObjectKeyFromObject(&p)
			Expect(err).To(BeNil())

			Expect(c.Get(context.TODO(), key, &p)).To(Succeed())
		}

		By("failing to get the unwatched pods")
		for _, p := range unwatchedPods {
			key, err := client.ObjectKeyFromObject(&p)
			Expect(err).To(BeNil())

			Expect(c.Get(context.TODO(), key, &p)).To(Succeed())
		}
		close(done)
		wg.Wait()
	})
})

func createPods(ctx context.Context, count int) ([]v1.Pod, error) {
	cl, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}

	pods := make([]v1.Pod, count)
	for i := 0; i < count; i++ {
		nsName := fmt.Sprintf("watch-%s", rand.String(5))
		ns := v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
		}
		pod := v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: nsName,
			},
			Spec: v1.PodSpec{Containers: []v1.Container{
				{Name: "test", Image: "test"},
			}},
		}
		if err := cl.Create(ctx, &ns); err != nil {
			return nil, err
		}
		if err := cl.Create(ctx, &pod); err != nil {
			return nil, err
		}
		pods[i] = pod
	}
	return pods, nil
}

func getNamespaces(objs []v1.Pod) (namespaces []string) {
	for _, obj := range objs {
		namespaces = append(namespaces, obj.GetNamespace())
	}
	return namespaces
}