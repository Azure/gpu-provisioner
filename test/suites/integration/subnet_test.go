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

package integration_test

import (
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter/pkg/apis/settings"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	awstest "github.com/aws/karpenter/pkg/test"
)

var _ = Describe("Subnets", func() {
	It("should use the subnet-id selector", func() {
		subnets := env.GetSubnets(map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName})
		Expect(len(subnets)).ToNot(Equal(0))
		shuffledAZs := lo.Shuffle(lo.Keys(subnets))
		firstSubnet := subnets[shuffledAZs[0]][0]

		provider := awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{
			AWS: v1alpha1.AWS{
				SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
				SubnetSelector:        map[string]string{"aws-ids": firstSubnet},
			},
		})
		provisioner := test.Provisioner(test.ProvisionerOptions{ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name}})
		pod := test.Pod()

		env.ExpectCreated(pod, provider, provisioner)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		env.ExpectInstance(pod.Spec.NodeName).To(HaveField("SubnetId", HaveValue(Equal(firstSubnet))))
	})
	It("should use resource based naming as node names", func() {
		subnets := env.GetSubnets(map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName})
		Expect(len(subnets)).ToNot(Equal(0))

		allSubnets := lo.Flatten(lo.Values(subnets))

		ExpectResourceBasedNamingEnabled(allSubnets...)
		DeferCleanup(func() {
			ExpectResourceBasedNamingDisabled(allSubnets...)
		})

		provider := awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{
			AWS: v1alpha1.AWS{
				SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
				SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			},
		})
		provisioner := test.Provisioner(test.ProvisionerOptions{ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name}})
		pod := test.Pod()

		env.ExpectCreated(pod, provider, provisioner)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		ExceptNodeNameToContainInstanceID(pod.Spec.NodeName)
	})
	It("should use the subnet tag selector with multiple tag values", func() {
		// Get all the subnets for the cluster
		subnets := env.GetSubnetNameAndIds(map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName})
		Expect(len(subnets)).To(BeNumerically(">", 1))
		firstSubnet := subnets[0]
		lastSubnet := subnets[len(subnets)-1]

		provider := awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{
			AWS: v1alpha1.AWS{
				SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
				SubnetSelector:        map[string]string{"Name": fmt.Sprintf("%s,%s", firstSubnet.Name, lastSubnet.Name)},
			},
		})
		provisioner := test.Provisioner(test.ProvisionerOptions{ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name}})
		pod := test.Pod()

		env.ExpectCreated(pod, provider, provisioner)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		env.ExpectInstance(pod.Spec.NodeName).To(HaveField("SubnetId", HaveValue(BeElementOf(firstSubnet.ID, lastSubnet.ID))))
	})

	It("should use a subnet within the AZ requested", func() {
		subnets := env.GetSubnets(map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName})
		Expect(len(subnets)).ToNot(Equal(0))
		shuffledAZs := lo.Shuffle(lo.Keys(subnets))

		provider := awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{
			AWS: v1alpha1.AWS{
				SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
				SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			},
		})
		provisioner := test.Provisioner(test.ProvisionerOptions{
			ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name},
			Requirements: []v1.NodeSelectorRequirement{
				{
					Key:      v1.LabelZoneFailureDomainStable,
					Operator: "In",
					Values:   []string{shuffledAZs[0]},
				},
			},
		})
		pod := test.Pod()

		env.ExpectCreated(pod, provider, provisioner)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		env.ExpectInstance(pod.Spec.NodeName).To(HaveField("SubnetId", Or(
			lo.Map(subnets[shuffledAZs[0]], func(subnetID string, _ int) types.GomegaMatcher { return HaveValue(Equal(subnetID)) })...,
		)))
	})

	It("should have the AWSNodeTemplateStatus for subnets", func() {
		provider := awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{
			AWS: v1alpha1.AWS{
				SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
				SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			},
		})

		env.ExpectCreated(provider)
		EventuallyExpectSubnets(provider)
	})
})

func ExpectResourceBasedNamingEnabled(subnetIDs ...string) {
	for subnetID := range subnetIDs {
		_, err := env.EC2API.ModifySubnetAttribute(&ec2.ModifySubnetAttributeInput{
			EnableResourceNameDnsARecordOnLaunch: &ec2.AttributeBooleanValue{
				Value: aws.Bool(true),
			},
			SubnetId: aws.String(subnetIDs[subnetID]),
		})
		Expect(err).To(BeNil())
		_, err = env.EC2API.ModifySubnetAttribute(&ec2.ModifySubnetAttributeInput{
			PrivateDnsHostnameTypeOnLaunch: aws.String("resource-name"),
			SubnetId:                       aws.String(subnetIDs[subnetID]),
		})
		Expect(err).To(BeNil())
	}
}

func ExpectResourceBasedNamingDisabled(subnetIDs ...string) {
	for subnetID := range subnetIDs {
		_, err := env.EC2API.ModifySubnetAttribute(&ec2.ModifySubnetAttributeInput{
			EnableResourceNameDnsARecordOnLaunch: &ec2.AttributeBooleanValue{
				Value: aws.Bool(false),
			},
			SubnetId: aws.String(subnetIDs[subnetID]),
		})
		Expect(err).To(BeNil())
		_, err = env.EC2API.ModifySubnetAttribute(&ec2.ModifySubnetAttributeInput{
			PrivateDnsHostnameTypeOnLaunch: aws.String("ip-name"),
			SubnetId:                       aws.String(subnetIDs[subnetID]),
		})
		Expect(err).To(BeNil())
	}
}

func ExceptNodeNameToContainInstanceID(nodeName string) {
	instance := env.GetInstance(nodeName)
	Expect(nodeName).To(Not(Equal(aws.StringValue(instance.InstanceId))))
	ContainSubstring(nodeName, aws.StringValue(instance.InstanceId))
}

// SubnetInfo is a simple struct for testing
type SubnetInfo struct {
	Name string
	ID   string
}

func EventuallyExpectSubnets(provider *v1alpha1.AWSNodeTemplate) {
	subnets := env.GetSubnets(map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName})
	Expect(len(subnets)).ToNot(Equal(0))
	subnetIDs := lo.Flatten(lo.Values(subnets))
	sort.Strings(subnetIDs)

	Eventually(func(g Gomega) {
		var ant v1alpha1.AWSNodeTemplate
		if err := env.Client.Get(env, client.ObjectKeyFromObject(provider), &ant); err != nil {
			return
		}
		subnetIDsInStatus := lo.Map(ant.Status.Subnets, func(subnet v1alpha1.Subnet, _ int) string {
			return subnet.ID
		})

		sort.Strings(subnetIDsInStatus)
		g.Expect(subnetIDsInStatus).To(Equal(subnetIDs))
	}).WithTimeout(10 * time.Second).Should(Succeed())
}
