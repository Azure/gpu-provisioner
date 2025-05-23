/*
       Copyright (c) Microsoft Corporation.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/transport"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/test"
)

func (env *Environment) ExpectCreatedWithOffset(offset int, objects ...client.Object) {
	for _, object := range objects {
		object.SetLabels(lo.Assign(object.GetLabels(), map[string]string{
			test.DiscoveryLabel: "unspecified",
		}))
		ExpectWithOffset(offset+1, env.Client.Create(env, object)).To(Succeed())
	}
}

func (env *Environment) ExpectCreated(objects ...client.Object) {
	env.ExpectCreatedWithOffset(1, objects...)
}

func (env *Environment) ExpectDeletedWithOffset(offset int, objects ...client.Object) {
	for _, object := range objects {
		ExpectWithOffset(offset+1, client.IgnoreNotFound(env.Client.Delete(env, object, client.PropagationPolicy(metav1.DeletePropagationForeground), &client.DeleteOptions{GracePeriodSeconds: ptr.Int64(0)}))).To(Succeed())
	}
}

func (env *Environment) ExpectDeleted(objects ...client.Object) {
	env.ExpectDeletedWithOffset(1, objects...)
}

// ExpectSettings gets the gpu-provisioner-global-settings ConfigMap
func (env *Environment) ExpectSettings() *v1.ConfigMap {
	GinkgoHelper()
	return env.ExpectConfigMapExists(types.NamespacedName{Namespace: "gpu-provisioner", Name: "gpu-provisioner-global-settings"})
}

func (env *Environment) ExpectConfigMapExists(key types.NamespacedName) *v1.ConfigMap {
	GinkgoHelper()
	cm := &v1.ConfigMap{}
	Expect(env.Client.Get(env, key, cm)).To(Succeed())
	return cm
}

func (env *Environment) ExpectExists(obj client.Object) {
	ExpectWithOffset(1, env.Client.Get(env, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
}

func (env *Environment) EventuallyExpectHealthy(pods ...*v1.Pod) {
	GinkgoHelper()
	env.EventuallyExpectHealthyWithTimeout(-1, pods...)
}

func (env *Environment) EventuallyExpectHealthyWithTimeout(timeout time.Duration, pods ...*v1.Pod) {
	GinkgoHelper()
	for _, pod := range pods {
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(env, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
			g.Expect(pod.Status.Conditions).To(ContainElement(And(
				HaveField("Type", Equal(v1.PodReady)),
				HaveField("Status", Equal(v1.ConditionTrue)),
			)))
		}).WithTimeout(timeout).Should(Succeed())
	}
}

func (env *Environment) EventuallyExpectGPUProvisionerRestarted() {
	GinkgoHelper()
	By("rolling out the new gpu-provisioner deployment")
	env.EventuallyExpectRollout("gpu-provisioner", "gpu-provisioner")

	By("waiting for a new gpu-provisioner pod to hold the lease")
	pods := env.ExpectGPUProvisionerPods()
	Eventually(func(g Gomega) {
		name := env.ExpectActiveGPUProvisionerPodName()
		g.Expect(lo.ContainsBy(pods, func(p *v1.Pod) bool {
			return p.Name == name
		})).To(BeTrue())
	}).Should(Succeed())
}

func (env *Environment) EventuallyExpectRollout(name, namespace string) {
	GinkgoHelper()
	By("restarting the deployment")
	deploy := &appsv1.Deployment{}
	Expect(env.Client.Get(env.Context, types.NamespacedName{Name: name, Namespace: namespace}, deploy)).To(Succeed())

	stored := deploy.DeepCopy()
	restartedAtAnnotation := map[string]string{
		"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339),
	}
	deploy.Spec.Template.Annotations = lo.Assign(deploy.Spec.Template.Annotations, restartedAtAnnotation)
	Expect(env.Client.Patch(env.Context, deploy, client.MergeFrom(stored))).To(Succeed())

	By("waiting for the newly generated deployment to rollout")
	Eventually(func(g Gomega) {
		podList := &v1.PodList{}
		g.Expect(env.Client.List(env.Context, podList, client.InNamespace(namespace))).To(Succeed())
		pods := lo.Filter(podList.Items, func(p v1.Pod, _ int) bool {
			return p.Annotations["kubectl.kubernetes.io/restartedAt"] == restartedAtAnnotation["kubectl.kubernetes.io/restartedAt"]
		})
		g.Expect(len(pods)).To(BeNumerically("==", lo.FromPtr(deploy.Spec.Replicas)))
		for _, pod := range pods {
			g.Expect(pod.Status.Conditions).To(ContainElement(And(
				HaveField("Type", Equal(v1.PodReady)),
				HaveField("Status", Equal(v1.ConditionTrue)),
			)))
			g.Expect(pod.Status.Phase).To(Equal(v1.PodRunning))
		}
	}).Should(Succeed())
}

func (env *Environment) ExpectGPUProvisionerPods() []*v1.Pod {
	GinkgoHelper()
	podList := &v1.PodList{}
	Expect(env.Client.List(env.Context, podList, client.MatchingLabels{
		"app.kubernetes.io/instance": "gpu-provisioner",
	})).To(Succeed())
	return lo.Map(podList.Items, func(p v1.Pod, _ int) *v1.Pod { return &p })
}

func (env *Environment) ExpectActiveGPUProvisionerPodName() string {
	GinkgoHelper()
	lease := &coordinationv1.Lease{}
	Expect(env.Client.Get(env.Context, types.NamespacedName{Name: "gpu-provisioner-leader-election", Namespace: "gpu-provisioner"}, lease)).To(Succeed())

	// Holder identity for lease is always in the format "<pod-name>_<pseudo-random-value>
	holderArr := strings.Split(lo.FromPtr(lease.Spec.HolderIdentity), "_")
	Expect(len(holderArr)).To(BeNumerically(">", 0))

	return holderArr[0]
}

func (env *Environment) ExpectActiveGPUProvisionerPod() *v1.Pod {
	GinkgoHelper()
	podName := env.ExpectActiveGPUProvisionerPodName()

	pod := &v1.Pod{}
	Expect(env.Client.Get(env.Context, types.NamespacedName{Name: podName, Namespace: "gpu-provisioner"}, pod)).To(Succeed())
	return pod
}

func (env *Environment) EventuallyExpectPendingPodCount(selector labels.Selector, numPods int) {
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(env.Monitor.PendingPodsCount(selector)).To(Equal(numPods))
	}).Should(Succeed())
}

func (env *Environment) EventuallyExpectHealthyPodCount(selector labels.Selector, numPods int) {
	By(fmt.Sprintf("waiting for %d pods matching selector %s to be ready", numPods, selector.String()))
	GinkgoHelper()
	env.EventuallyExpectHealthyPodCountWithTimeout(-1, selector, numPods)
}

func (env *Environment) EventuallyExpectHealthyPodCountWithTimeout(timeout time.Duration, selector labels.Selector, numPods int) {
	GinkgoHelper()
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(env.Monitor.RunningPodsCount(selector)).To(Equal(numPods))
	}).WithTimeout(timeout).Should(Succeed())
}

func (env *Environment) ExpectPodsMatchingSelector(selector labels.Selector) []*v1.Pod {
	GinkgoHelper()

	podList := &v1.PodList{}
	Expect(env.Client.List(env.Context, podList, client.MatchingLabelsSelector{Selector: selector})).To(Succeed())
	return lo.ToSlicePtr(podList.Items)
}

func (env *Environment) ExpectUniqueNodeNames(selector labels.Selector, uniqueNames int) {
	pods := env.Monitor.RunningPods(selector)
	nodeNames := sets.NewString()
	for _, pod := range pods {
		nodeNames.Insert(pod.Spec.NodeName)
	}
	ExpectWithOffset(1, len(nodeNames)).To(BeNumerically("==", uniqueNames))
}

func (env *Environment) EventuallyExpectNotFound(objects ...client.Object) {
	env.EventuallyExpectNotFoundWithOffset(1, objects...)
}

func (env *Environment) EventuallyExpectNotFoundWithOffset(offset int, objects ...client.Object) {
	env.EventuallyExpectNotFoundAssertionWithOffset(offset+1, objects...).Should(Succeed())
}

func (env *Environment) EventuallyExpectNotFoundAssertion(objects ...client.Object) AsyncAssertion {
	return env.EventuallyExpectNotFoundAssertionWithOffset(1, objects...)
}

func (env *Environment) EventuallyExpectNotFoundAssertionWithOffset(offset int, objects ...client.Object) AsyncAssertion {
	return EventuallyWithOffset(offset, func(g Gomega) {
		for _, object := range objects {
			err := env.Client.Get(env, client.ObjectKeyFromObject(object), object)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}
	})
}

func (env *Environment) ExpectCreatedNodeCount(comparator string, count int) []*v1.Node {
	createdNodes := env.Monitor.CreatedNodes()
	ExpectWithOffset(1, len(createdNodes)).To(BeNumerically(comparator, count),
		fmt.Sprintf("expected %d created nodes, had %d (%v)", count, len(createdNodes), NodeNames(createdNodes)))
	return createdNodes
}

func NodeNames(nodes []*v1.Node) []string {
	return lo.Map(nodes, func(n *v1.Node, index int) string {
		return n.Name
	})
}

func (env *Environment) EventuallyExpectNodeCount(comparator string, count int) []*v1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for nodes to be %s to %d", comparator, count))
	nodeList := &v1.NodeList{}
	Eventually(func(g Gomega) {
		g.Expect(env.Client.List(env, nodeList, client.HasLabels{test.DiscoveryLabel})).To(Succeed())
		g.Expect(len(nodeList.Items)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d nodes, had %d (%v)", count, len(nodeList.Items), NodeNames(lo.ToSlicePtr(nodeList.Items))))
	}).Should(Succeed())
	return lo.ToSlicePtr(nodeList.Items)
}

func (env *Environment) EventuallyExpectNodeCountWithSelector(comparator string, count int, selector labels.Selector) []*v1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for nodes with selector %v to be %s to %d", selector, comparator, count))
	nodeList := &v1.NodeList{}
	Eventually(func(g Gomega) {
		g.Expect(env.Client.List(env, nodeList, client.HasLabels{test.DiscoveryLabel}, client.MatchingLabelsSelector{Selector: selector})).To(Succeed())
		g.Expect(len(nodeList.Items)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d nodes, had %d (%v)", count, len(nodeList.Items), NodeNames(lo.ToSlicePtr(nodeList.Items))))
	}).Should(Succeed())
	return lo.ToSlicePtr(nodeList.Items)
}

func (env *Environment) EventuallyExpectCreatedNodeCount(comparator string, count int) []*v1.Node {
	By(fmt.Sprintf("waiting for created nodes to be %s to %d", comparator, count))
	var createdNodes []*v1.Node
	EventuallyWithOffset(1, func(g Gomega) {
		createdNodes = env.Monitor.CreatedNodes()
		g.Expect(len(createdNodes)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d created nodes, had %d (%v)", count, len(createdNodes), NodeNames(createdNodes)))
	}).Should(Succeed())
	return createdNodes
}

func (env *Environment) EventuallyExpectDeletedNodeCount(comparator string, count int) []*v1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for deleted nodes to be %s to %d", comparator, count))
	var deletedNodes []*v1.Node
	Eventually(func(g Gomega) {
		deletedNodes = env.Monitor.DeletedNodes()
		g.Expect(len(deletedNodes)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d deleted nodes, had %d (%v)", count, len(deletedNodes), NodeNames(deletedNodes)))
	}).Should(Succeed())
	return deletedNodes
}

func (env *Environment) EventuallyExpectDeletedNodeCountWithSelector(comparator string, count int, selector labels.Selector) []*v1.Node {
	GinkgoHelper()
	By(fmt.Sprintf("waiting for deleted nodes with selector %v to be %s to %d", selector, comparator, count))
	var deletedNodes []*v1.Node
	Eventually(func(g Gomega) {
		deletedNodes = env.Monitor.DeletedNodes()
		deletedNodes = lo.Filter(deletedNodes, func(n *v1.Node, _ int) bool {
			return selector.Matches(labels.Set(n.Labels))
		})
		g.Expect(len(deletedNodes)).To(BeNumerically(comparator, count),
			fmt.Sprintf("expected %d deleted nodes, had %d (%v)", count, len(deletedNodes), NodeNames(deletedNodes)))
	}).Should(Succeed())
	return deletedNodes
}

func (env *Environment) EventuallyExpectInitializedNodeCount(comparator string, count int) []*v1.Node {
	By(fmt.Sprintf("waiting for initialized nodes to be %s to %d", comparator, count))
	var nodes []*v1.Node
	EventuallyWithOffset(1, func(g Gomega) {
		nodes = env.Monitor.CreatedNodes()
		nodes = lo.Filter(nodes, func(n *v1.Node, _ int) bool {
			return n.Labels[karpenterv1.NodeInitializedLabelKey] == "true"
		})
		g.Expect(len(nodes)).To(BeNumerically(comparator, count))
	}).Should(Succeed())
	return nodes
}

func (env *Environment) EventuallyExpectCreatedNodeClaimCount(comparator string, count int) []*karpenterv1.NodeClaim {
	By(fmt.Sprintf("waiting for created node claims to be %s to %d", comparator, count))
	nodeClaimList := &karpenterv1.NodeClaimList{}
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(env.Client.List(env.Context, nodeClaimList)).To(Succeed())
		g.Expect(len(nodeClaimList.Items)).To(BeNumerically(comparator, count))
	}).Should(Succeed())
	return lo.Map(nodeClaimList.Items, func(nc karpenterv1.NodeClaim, _ int) *karpenterv1.NodeClaim {
		return &nc
	})
}

func (env *Environment) EventuallyExpectNodeClaimsReady(objects ...client.Object) {
	Eventually(func(g Gomega) {
		for _, object := range objects {
			temp := &karpenterv1.NodeClaim{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(object), temp)).Should(Succeed())
			g.Expect(temp.StatusConditions().Root().IsTrue()).To(BeTrue())
		}
	}).Should(Succeed())
}

func (env *Environment) GetNode(nodeName string) v1.Node {
	var node v1.Node
	ExpectWithOffset(1, env.Client.Get(env.Context, types.NamespacedName{Name: nodeName}, &node)).To(Succeed())
	return node
}

func (env *Environment) ExpectNoCrashes() {
	_, crashed := lo.Find(lo.Values(env.Monitor.RestartCount()), func(restartCount int) bool {
		return restartCount > 0
	})
	ExpectWithOffset(1, crashed).To(BeFalse(), "expected gpu-provisioner containers to not crash")
}

var (
	lastLogged = metav1.Now()
)

func (env *Environment) printControllerLogs(options *v1.PodLogOptions) {
	fmt.Println("------- START CONTROLLER LOGS -------")
	defer fmt.Println("------- END CONTROLLER LOGS -------")

	if options.SinceTime == nil {
		options.SinceTime = lastLogged.DeepCopy()
		lastLogged = metav1.Now()
	}
	pods := env.ExpectGPUProvisionerPods()
	for _, pod := range pods {
		temp := options.DeepCopy() // local version of the log options

		fmt.Printf("------- pod/%s -------\n", pod.Name)
		if pod.Status.ContainerStatuses[0].RestartCount > 0 {
			fmt.Printf("[PREVIOUS CONTAINER LOGS]\n")
			temp.Previous = true
		}
		stream, err := env.KubeClient.CoreV1().Pods("gpu-provisioner").GetLogs(pod.Name, temp).Stream(env.Context)
		if err != nil {
			logging.FromContext(env.Context).Errorf("fetching controller logs: %s", err)
			return
		}
		log := &bytes.Buffer{}
		_, err = io.Copy(log, stream)
		Expect(err).ToNot(HaveOccurred())
		logging.FromContext(env.Context).Info(log)
	}
}

func (env *Environment) ExpectCABundle() string {
	// Discover CA Bundle from the REST client. We could alternatively
	// have used the simpler client-go InClusterConfig() method.
	// However, that only works when Karpenter is running as a Pod
	// within the same cluster it's managing.
	transportConfig, err := env.Config.TransportConfig()
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	_, err = transport.TLSConfigFor(transportConfig) // fills in CAData!
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	logging.FromContext(env.Context).Debugf("Discovered caBundle, length %d", len(transportConfig.TLS.CAData))
	return base64.StdEncoding.EncodeToString(transportConfig.TLS.CAData)
}
