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

package interruption

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"knative.dev/pkg/ptr"

	"github.com/Azure/karpenter/test/pkg/environment/aws"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter/pkg/apis/settings"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	"github.com/aws/karpenter/pkg/controllers/interruption/messages"
	"github.com/aws/karpenter/pkg/controllers/interruption/messages/scheduledchange"
	awstest "github.com/aws/karpenter/pkg/test"
	"github.com/aws/karpenter/pkg/utils"
)

var env *aws.Environment
var provider *v1alpha1.AWSNodeTemplate

func TestInterruption(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = aws.NewEnvironment(t)
	})
	RunSpecs(t, "Interruption")
}

var _ = BeforeEach(func() {
	env.BeforeEach()
	env.ExpectQueueExists()
})
var _ = AfterEach(func() { env.Cleanup() })
var _ = AfterEach(func() { env.AfterEach() })

var _ = Describe("Interruption", Label("AWS"), func() {
	It("should terminate the spot instance and spin-up a new node on spot interruption warning", func() {
		By("Creating a single healthy node with a healthy deployment")
		provider = awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{AWS: v1alpha1.AWS{
			SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
		}})
		provisioner := test.Provisioner(test.ProvisionerOptions{
			Requirements: []v1.NodeSelectorRequirement{
				{
					Key:      v1alpha5.LabelCapacityType,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{v1alpha5.CapacityTypeSpot},
				},
			},
			ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name},
		})
		numPods := 1
		dep := test.Deployment(test.DeploymentOptions{
			Replicas: int32(numPods),
			PodOptions: test.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "my-app"},
				},
				TerminationGracePeriodSeconds: ptr.Int64(0),
			},
		})
		selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)

		env.ExpectCreated(provider, provisioner, dep)

		env.EventuallyExpectHealthyPodCount(selector, numPods)
		env.ExpectCreatedNodeCount("==", 1)

		node := env.Monitor.CreatedNodes()[0]
		instanceID, err := utils.ParseInstanceID(node.Spec.ProviderID)
		Expect(err).ToNot(HaveOccurred())

		By("interrupting the spot instance")
		exp := env.ExpectSpotInterruptionExperiment(instanceID)
		DeferCleanup(func() {
			env.ExpectExperimentTemplateDeleted(*exp.ExperimentTemplateId)
		})

		// We are expecting the node to be terminated before the termination is complete
		By("waiting to receive the interruption and terminate the node")
		env.EventuallyExpectNotFoundAssertion(node).WithTimeout(time.Second * 110).Should(Succeed())
		env.EventuallyExpectHealthyPodCount(selector, 1)
	})
	It("should terminate the node at the API server when the EC2 instance is stopped", func() {
		By("Creating a single healthy node with a healthy deployment")
		provider = awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{AWS: v1alpha1.AWS{
			SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
		}})
		provisioner := test.Provisioner(test.ProvisionerOptions{
			Requirements: []v1.NodeSelectorRequirement{
				{
					Key:      v1alpha5.LabelCapacityType,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{v1alpha5.CapacityTypeOnDemand},
				},
			},
			ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name},
		})
		numPods := 1
		dep := test.Deployment(test.DeploymentOptions{
			Replicas: int32(numPods),
			PodOptions: test.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "my-app"},
				},
				TerminationGracePeriodSeconds: ptr.Int64(0),
			},
		})
		selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)

		env.ExpectCreated(provider, provisioner, dep)

		env.EventuallyExpectHealthyPodCount(selector, numPods)
		env.ExpectCreatedNodeCount("==", 1)

		node := env.Monitor.CreatedNodes()[0]

		By("Stopping the EC2 instance without the EKS cluster's knowledge")
		env.ExpectInstanceStopped(node.Name)                                                   // Make a call to the EC2 api to stop the instance
		env.EventuallyExpectNotFoundAssertion(node).WithTimeout(time.Minute).Should(Succeed()) // shorten the timeout since we should react faster
		env.EventuallyExpectHealthyPodCount(selector, 1)
	})
	It("should terminate the node at the API server when the EC2 instance is terminated", func() {
		By("Creating a single healthy node with a healthy deployment")
		provider = awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{AWS: v1alpha1.AWS{
			SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
		}})
		provisioner := test.Provisioner(test.ProvisionerOptions{
			Requirements: []v1.NodeSelectorRequirement{
				{
					Key:      v1alpha5.LabelCapacityType,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{v1alpha5.CapacityTypeOnDemand},
				},
			},
			ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name},
		})
		numPods := 1
		dep := test.Deployment(test.DeploymentOptions{
			Replicas: int32(numPods),
			PodOptions: test.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "my-app"},
				},
				TerminationGracePeriodSeconds: ptr.Int64(0),
			},
		})
		selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)

		env.ExpectCreated(provider, provisioner, dep)

		env.EventuallyExpectHealthyPodCount(selector, numPods)
		env.ExpectCreatedNodeCount("==", 1)

		node := env.Monitor.CreatedNodes()[0]

		By("Terminating the EC2 instance without the EKS cluster's knowledge")
		env.ExpectInstanceTerminated(node.Name)                                                // Make a call to the EC2 api to stop the instance
		env.EventuallyExpectNotFoundAssertion(node).WithTimeout(time.Minute).Should(Succeed()) // shorten the timeout since we should react faster
		env.EventuallyExpectHealthyPodCount(selector, 1)
	})
	It("should terminate the node when receiving a scheduled change health event", func() {
		By("Creating a single healthy node with a healthy deployment")
		provider = awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{AWS: v1alpha1.AWS{
			SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
		}})
		provisioner := test.Provisioner(test.ProvisionerOptions{
			Requirements: []v1.NodeSelectorRequirement{
				{
					Key:      v1alpha5.LabelCapacityType,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{v1alpha5.CapacityTypeOnDemand},
				},
			},
			ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name},
		})
		numPods := 1
		dep := test.Deployment(test.DeploymentOptions{
			Replicas: int32(numPods),
			PodOptions: test.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "my-app"},
				},
				TerminationGracePeriodSeconds: ptr.Int64(0),
			},
		})
		selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)

		env.ExpectCreated(provider, provisioner, dep)

		env.EventuallyExpectHealthyPodCount(selector, numPods)
		env.ExpectCreatedNodeCount("==", 1)

		node := env.Monitor.CreatedNodes()[0]
		instanceID, err := utils.ParseInstanceID(node.Spec.ProviderID)
		Expect(err).ToNot(HaveOccurred())

		By("Creating a scheduled change health event in the SQS message queue")
		env.ExpectMessagesCreated(scheduledChangeMessage(env.Region, "000000000000", instanceID))
		env.EventuallyExpectNotFoundAssertion(node).WithTimeout(time.Minute).Should(Succeed()) // shorten the timeout since we should react faster
		env.EventuallyExpectHealthyPodCount(selector, 1)
	})
})

func scheduledChangeMessage(region, accountID, involvedInstanceID string) scheduledchange.Message {
	return scheduledchange.Message{
		Metadata: messages.Metadata{
			Version:    "0",
			Account:    accountID,
			DetailType: "AWS Health Event",
			ID:         string(uuid.NewUUID()),
			Region:     region,
			Resources: []string{
				fmt.Sprintf("arn:aws:ec2:%s:instance/%s", region, involvedInstanceID),
			},
			Source: "aws.health",
			Time:   time.Now(),
		},
		Detail: scheduledchange.Detail{
			Service:           "EC2",
			EventTypeCategory: "scheduledChange",
			AffectedEntities: []scheduledchange.AffectedEntity{
				{
					EntityValue: involvedInstanceID,
				},
			},
		},
	}
}
