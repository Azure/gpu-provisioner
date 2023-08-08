/*
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
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/transport"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	pscheduling "github.com/aws/karpenter-core/pkg/controllers/provisioning/scheduling"
	"github.com/aws/karpenter-core/pkg/scheduling"
	"github.com/aws/karpenter-core/pkg/test"
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
		ExpectWithOffset(offset+1, env.Client.Delete(env, object, client.PropagationPolicy(metav1.DeletePropagationForeground), &client.DeleteOptions{GracePeriodSeconds: ptr.Int64(0)})).To(Succeed())
	}
}

func (env *Environment) ExpectDeleted(objects ...client.Object) {
	env.ExpectDeletedWithOffset(1, objects...)
}

func (env *Environment) ExpectUpdatedWithOffset(offset int, objects ...client.Object) {
	for _, o := range objects {
		current := o.DeepCopyObject().(client.Object)
		ExpectWithOffset(offset+1, env.Client.Get(env.Context, client.ObjectKeyFromObject(current), current)).To(Succeed())
		o.SetResourceVersion(current.GetResourceVersion())
		ExpectWithOffset(offset+1, env.Client.Update(env.Context, o)).To(Succeed())
	}
}

func (env *Environment) ExpectUpdated(objects ...client.Object) {
	env.ExpectUpdatedWithOffset(1, objects...)
}

func (env *Environment) ExpectCreatedOrUpdated(objects ...client.Object) {
	for _, o := range objects {
		current := o.DeepCopyObject().(client.Object)
		err := env.Client.Get(env, client.ObjectKeyFromObject(current), current)
		if err != nil {
			if errors.IsNotFound(err) {
				env.ExpectCreatedWithOffset(1, o)
			} else {
				Fail(fmt.Sprintf("Getting object %s, %v", client.ObjectKeyFromObject(o), err))
			}
		} else {
			env.ExpectUpdatedWithOffset(1, o)
		}
	}
}

// ExpectSettings gets the karpenter-global-settings ConfigMap
func (env *Environment) ExpectSettings() *v1.ConfigMap {
	GinkgoHelper()
	return env.ExpectConfigMapExists(types.NamespacedName{Namespace: "karpenter", Name: "karpenter-global-settings"})
}

// ExpectSettingsReplaced performs a full replace of the settings, replacing the existing data
// with the data passed through
func (env *Environment) ExpectSettingsReplaced(data ...map[string]string) {
	GinkgoHelper()
	if env.ExpectConfigMapDataReplaced(types.NamespacedName{Namespace: "karpenter", Name: "karpenter-global-settings"}, data...) {
		env.EventuallyExpectKarpenterRestarted()
	}
}

// ExpectSettingsOverridden overrides specific values specified through data. It only overrides
// or inserts the specific values specified and does not upsert any of the existing data
func (env *Environment) ExpectSettingsOverridden(data ...map[string]string) {
	GinkgoHelper()
	if env.ExpectConfigMapDataOverridden(types.NamespacedName{Namespace: "karpenter", Name: "karpenter-global-settings"}, data...) {
		env.EventuallyExpectKarpenterRestarted()
	}
}

func (env *Environment) ExpectConfigMapExists(key types.NamespacedName) *v1.ConfigMap {
	GinkgoHelper()
	cm := &v1.ConfigMap{}
	Expect(env.Client.Get(env, key, cm)).To(Succeed())
	return cm
}

func (env *Environment) ExpectConfigMapDataReplaced(key types.NamespacedName, data ...map[string]string) (changed bool) {
	GinkgoHelper()
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}
	err := env.Client.Get(env, key, cm)
	Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

	stored := cm.DeepCopy()
	cm.Data = lo.Assign(data...) // Completely replace the data

	// If the data hasn't changed, we can just return and not update anything
	if equality.Semantic.DeepEqual(stored, cm) {
		return false
	}
	// Update the configMap to update the settings
	env.ExpectCreatedOrUpdated(cm)
	return true
}

func (env *Environment) ExpectConfigMapDataOverridden(key types.NamespacedName, data ...map[string]string) (changed bool) {
	GinkgoHelper()
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}
	err := env.Client.Get(env, key, cm)
	Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

	stored := cm.DeepCopy()
	cm.Data = lo.Assign(append([]map[string]string{cm.Data}, data...)...)

	// If the data hasn't changed, we can just return and not update anything
	if equality.Semantic.DeepEqual(stored, cm) {
		return false
	}
	// Update the configMap to update the settings
	env.ExpectCreatedOrUpdated(cm)
	return true
}

func (env *Environment) ExpectPodENIEnabled() {
	env.ExpectDaemonSetEnvironmentVariableUpdatedWithOffset(1, types.NamespacedName{Namespace: "kube-system", Name: "aws-node"},
		"ENABLE_POD_ENI", "true")
}

func (env *Environment) ExpectPodENIDisabled() {
	env.ExpectDaemonSetEnvironmentVariableUpdatedWithOffset(1, types.NamespacedName{Namespace: "kube-system", Name: "aws-node"},
		"ENABLE_POD_ENI", "false")
}

func (env *Environment) ExpectPrefixDelegationEnabled() {
	env.ExpectDaemonSetEnvironmentVariableUpdatedWithOffset(1, types.NamespacedName{Namespace: "kube-system", Name: "aws-node"},
		"ENABLE_PREFIX_DELEGATION", "true")
}

func (env *Environment) ExpectPrefixDelegationDisabled() {
	env.ExpectDaemonSetEnvironmentVariableUpdatedWithOffset(1, types.NamespacedName{Namespace: "kube-system", Name: "aws-node"},
		"ENABLE_PREFIX_DELEGATION", "false")
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

func (env *Environment) EventuallyExpectKarpenterRestarted() {
	GinkgoHelper()
	By("rolling out the new karpenter deployment")
	env.EventuallyExpectRollout("karpenter", "karpenter")

	By("waiting for a new karpenter pod to hold the lease")
	pods := env.ExpectKarpenterPods()
	Eventually(func(g Gomega) {
		name := env.ExpectActiveKarpenterPodName()
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

func (env *Environment) ExpectKarpenterPods() []*v1.Pod {
	GinkgoHelper()
	podList := &v1.PodList{}
	Expect(env.Client.List(env.Context, podList, client.MatchingLabels{
		"app.kubernetes.io/instance": "karpenter",
	})).To(Succeed())
	return lo.Map(podList.Items, func(p v1.Pod, _ int) *v1.Pod { return &p })
}

func (env *Environment) ExpectActiveKarpenterPodName() string {
	GinkgoHelper()
	lease := &coordinationv1.Lease{}
	Expect(env.Client.Get(env.Context, types.NamespacedName{Name: "karpenter-leader-election", Namespace: "karpenter"}, lease)).To(Succeed())

	// Holder identity for lease is always in the format "<pod-name>_<pseudo-random-value>
	holderArr := strings.Split(lo.FromPtr(lease.Spec.HolderIdentity), "_")
	Expect(len(holderArr)).To(BeNumerically(">", 0))

	return holderArr[0]
}

func (env *Environment) ExpectActiveKarpenterPod() *v1.Pod {
	GinkgoHelper()
	podName := env.ExpectActiveKarpenterPodName()

	pod := &v1.Pod{}
	Expect(env.Client.Get(env.Context, types.NamespacedName{Name: podName, Namespace: "karpenter"}, pod)).To(Succeed())
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

func (env *Environment) eventuallyExpectScaleDown() {
	EventuallyWithOffset(1, func(g Gomega) {
		// expect the current node count to be what it was when the test started
		g.Expect(env.Monitor.NodeCount()).To(Equal(env.StartingNodeCount))
	}).Should(Succeed(), fmt.Sprintf("expected scale down to %d nodes, had %d", env.StartingNodeCount, env.Monitor.NodeCount()))
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
			return n.Labels[v1alpha5.LabelNodeInitialized] == "true"
		})
		g.Expect(len(nodes)).To(BeNumerically(comparator, count))
	}).Should(Succeed())
	return nodes
}

func (env *Environment) EventuallyExpectCreatedMachineCount(comparator string, count int) []*v1alpha5.Machine {
	By(fmt.Sprintf("waiting for created machines to be %s to %d", comparator, count))
	machineList := &v1alpha5.MachineList{}
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(env.Client.List(env.Context, machineList)).To(Succeed())
		g.Expect(len(machineList.Items)).To(BeNumerically(comparator, count))
	}).Should(Succeed())
	return lo.Map(machineList.Items, func(m v1alpha5.Machine, _ int) *v1alpha5.Machine {
		return &m
	})
}

func (env *Environment) EventuallyExpectMachinesReady(machines ...*v1alpha5.Machine) {
	Eventually(func(g Gomega) {
		for _, machine := range machines {
			temp := &v1alpha5.Machine{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(machine), temp)).Should(Succeed())
			g.Expect(temp.StatusConditions().IsHappy()).To(BeTrue())
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
	ExpectWithOffset(1, crashed).To(BeFalse(), "expected karpenter containers to not crash")
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
	pods := env.ExpectKarpenterPods()
	for _, pod := range pods {
		temp := options.DeepCopy() // local version of the log options

		fmt.Printf("------- pod/%s -------\n", pod.Name)
		if pod.Status.ContainerStatuses[0].RestartCount > 0 {
			fmt.Printf("[PREVIOUS CONTAINER LOGS]\n")
			temp.Previous = true
		}
		stream, err := env.KubeClient.CoreV1().Pods("karpenter").GetLogs(pod.Name, temp).Stream(env.Context)
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

func (env *Environment) EventuallyExpectMinUtilization(resource v1.ResourceName, comparator string, value float64) {
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(env.Monitor.MinUtilization(resource)).To(BeNumerically(comparator, value))
	}).Should(Succeed())
}

func (env *Environment) EventuallyExpectAvgUtilization(resource v1.ResourceName, comparator string, value float64) {
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(env.Monitor.AvgUtilization(resource)).To(BeNumerically(comparator, value))
	}, 10*time.Minute).Should(Succeed())
}

func (env *Environment) ExpectDaemonSetEnvironmentVariableUpdated(obj client.ObjectKey, name, value string) {
	env.ExpectDaemonSetEnvironmentVariableUpdatedWithOffset(1, obj, name, value)
}

func (env *Environment) ExpectDaemonSetEnvironmentVariableUpdatedWithOffset(offset int, obj client.ObjectKey, name, value string) {
	ds := &appsv1.DaemonSet{}
	ExpectWithOffset(offset+1, env.Client.Get(env.Context, obj, ds)).To(Succeed())
	ExpectWithOffset(offset+1, len(ds.Spec.Template.Spec.Containers)).To(BeNumerically("==", 1))
	patch := client.MergeFrom(ds.DeepCopy())

	// If the value is found, update it. Else, create it
	found := false
	for i, v := range ds.Spec.Template.Spec.Containers[0].Env {
		if v.Name == name {
			ds.Spec.Template.Spec.Containers[0].Env[i].Value = value
			found = true
		}
	}
	if !found {
		ds.Spec.Template.Spec.Containers[0].Env = append(ds.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
			Name:  name,
			Value: value,
		})
	}
	ExpectWithOffset(offset+1, env.Client.Patch(env.Context, ds, patch)).To(Succeed())
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

func (env *Environment) GetDaemonSetCount(prov *v1alpha5.Provisioner) int {
	// Performs the same logic as the scheduler to get the number of daemonset
	// pods that we estimate we will need to schedule as overhead to each node
	daemonSetList := &appsv1.DaemonSetList{}
	Expect(env.Client.List(env.Context, daemonSetList)).To(Succeed())

	return lo.CountBy(daemonSetList.Items, func(d appsv1.DaemonSet) bool {
		p := &v1.Pod{Spec: d.Spec.Template.Spec}
		nodeTemplate := pscheduling.NewMachineTemplate(prov)
		if err := nodeTemplate.Taints.Tolerates(p); err != nil {
			return false
		}
		if err := nodeTemplate.Requirements.Compatible(scheduling.NewPodRequirements(p)); err != nil {
			return false
		}
		return true
	})
}
