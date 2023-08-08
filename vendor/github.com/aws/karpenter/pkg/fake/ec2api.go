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

package fake

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/samber/lo"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/utils/sets"

	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter-core/pkg/utils/atomic"
)

type CapacityPool struct {
	CapacityType string
	InstanceType string
	Zone         string
}

// EC2Behavior must be reset between tests otherwise tests will
// pollute each other.
type EC2Behavior struct {
	DescribeImagesOutput                AtomicPtr[ec2.DescribeImagesOutput]
	DescribeLaunchTemplatesOutput       AtomicPtr[ec2.DescribeLaunchTemplatesOutput]
	DescribeSubnetsOutput               AtomicPtr[ec2.DescribeSubnetsOutput]
	DescribeSecurityGroupsOutput        AtomicPtr[ec2.DescribeSecurityGroupsOutput]
	DescribeInstanceTypesOutput         AtomicPtr[ec2.DescribeInstanceTypesOutput]
	DescribeInstanceTypeOfferingsOutput AtomicPtr[ec2.DescribeInstanceTypeOfferingsOutput]
	DescribeAvailabilityZonesOutput     AtomicPtr[ec2.DescribeAvailabilityZonesOutput]
	DescribeSpotPriceHistoryInput       AtomicPtr[ec2.DescribeSpotPriceHistoryInput]
	DescribeSpotPriceHistoryOutput      AtomicPtr[ec2.DescribeSpotPriceHistoryOutput]
	CreateFleetBehavior                 MockedFunction[ec2.CreateFleetInput, ec2.CreateFleetOutput]
	TerminateInstancesBehavior          MockedFunction[ec2.TerminateInstancesInput, ec2.TerminateInstancesOutput]
	DescribeInstancesBehavior           MockedFunction[ec2.DescribeInstancesInput, ec2.DescribeInstancesOutput]
	CreateTagsBehavior                  MockedFunction[ec2.CreateTagsInput, ec2.CreateTagsOutput]
	CalledWithCreateLaunchTemplateInput AtomicPtrSlice[ec2.CreateLaunchTemplateInput]
	CalledWithDescribeImagesInput       AtomicPtrSlice[ec2.DescribeImagesInput]
	Instances                           sync.Map
	LaunchTemplates                     sync.Map
	InsufficientCapacityPools           atomic.Slice[CapacityPool]
	NextError                           AtomicError
}

type EC2API struct {
	ec2iface.EC2API
	EC2Behavior
}

// DefaultSupportedUsageClasses is a var because []*string can't be a const
var DefaultSupportedUsageClasses = aws.StringSlice([]string{"on-demand", "spot"})

// Reset must be called between tests otherwise tests will pollute
// each other.
func (e *EC2API) Reset() {
	e.DescribeImagesOutput.Reset()
	e.DescribeLaunchTemplatesOutput.Reset()
	e.DescribeSubnetsOutput.Reset()
	e.DescribeSecurityGroupsOutput.Reset()
	e.DescribeInstanceTypesOutput.Reset()
	e.DescribeInstanceTypeOfferingsOutput.Reset()
	e.DescribeAvailabilityZonesOutput.Reset()
	e.CreateFleetBehavior.Reset()
	e.TerminateInstancesBehavior.Reset()
	e.DescribeInstancesBehavior.Reset()
	e.CalledWithCreateLaunchTemplateInput.Reset()
	e.CalledWithDescribeImagesInput.Reset()
	e.DescribeSpotPriceHistoryInput.Reset()
	e.DescribeSpotPriceHistoryOutput.Reset()
	e.Instances.Range(func(k, v any) bool {
		e.Instances.Delete(k)
		return true
	})
	e.LaunchTemplates.Range(func(k, v any) bool {
		e.LaunchTemplates.Delete(k)
		return true
	})
	e.InsufficientCapacityPools.Reset()
	e.NextError.Reset()
}

// nolint: gocyclo
func (e *EC2API) CreateFleetWithContext(_ context.Context, input *ec2.CreateFleetInput, _ ...request.Option) (*ec2.CreateFleetOutput, error) {
	return e.CreateFleetBehavior.Invoke(input, func(input *ec2.CreateFleetInput) (*ec2.CreateFleetOutput, error) {
		if input.LaunchTemplateConfigs[0].LaunchTemplateSpecification.LaunchTemplateName == nil {
			return nil, fmt.Errorf("missing launch template name")
		}
		var instanceIds []*string
		var skippedPools []CapacityPool
		var spotInstanceRequestID *string

		if aws.StringValue(input.TargetCapacitySpecification.DefaultTargetCapacityType) == v1alpha5.CapacityTypeSpot {
			spotInstanceRequestID = aws.String(test.RandomName())
		}

		fulfilled := 0
		for _, ltc := range input.LaunchTemplateConfigs {
			for _, override := range ltc.Overrides {
				skipInstance := false
				e.InsufficientCapacityPools.Range(func(pool CapacityPool) bool {
					if pool.InstanceType == aws.StringValue(override.InstanceType) &&
						pool.Zone == aws.StringValue(override.AvailabilityZone) &&
						pool.CapacityType == aws.StringValue(input.TargetCapacitySpecification.DefaultTargetCapacityType) {
						skippedPools = append(skippedPools, pool)
						skipInstance = true
						return false
					}
					return true
				})
				if skipInstance {
					continue
				}
				amiID := aws.String("")
				if e.CalledWithCreateLaunchTemplateInput.Len() > 0 {
					lt := e.CalledWithCreateLaunchTemplateInput.Pop()
					amiID = lt.LaunchTemplateData.ImageId
					e.CalledWithCreateLaunchTemplateInput.Add(lt)
				}
				instanceState := ec2.InstanceStateNameRunning
				for ; fulfilled < int(*input.TargetCapacitySpecification.TotalTargetCapacity); fulfilled++ {
					instance := &ec2.Instance{
						ImageId:               aws.String(*amiID),
						InstanceId:            aws.String(test.RandomName()),
						Placement:             &ec2.Placement{AvailabilityZone: input.LaunchTemplateConfigs[0].Overrides[0].AvailabilityZone},
						PrivateDnsName:        aws.String(randomdata.IpV4Address()),
						InstanceType:          input.LaunchTemplateConfigs[0].Overrides[0].InstanceType,
						SpotInstanceRequestId: spotInstanceRequestID,
						State: &ec2.InstanceState{
							Name: &instanceState,
						},
					}
					e.Instances.Store(*instance.InstanceId, instance)
					instanceIds = append(instanceIds, instance.InstanceId)
				}
			}
			if fulfilled == int(*input.TargetCapacitySpecification.TotalTargetCapacity) {
				break
			}
		}
		result := &ec2.CreateFleetOutput{Instances: []*ec2.CreateFleetInstance{
			{
				InstanceIds:  instanceIds,
				InstanceType: input.LaunchTemplateConfigs[0].Overrides[0].InstanceType,
				Lifecycle:    input.TargetCapacitySpecification.DefaultTargetCapacityType,
				LaunchTemplateAndOverrides: &ec2.LaunchTemplateAndOverridesResponse{
					Overrides: &ec2.FleetLaunchTemplateOverrides{
						SubnetId:         input.LaunchTemplateConfigs[0].Overrides[0].SubnetId,
						InstanceType:     input.LaunchTemplateConfigs[0].Overrides[0].InstanceType,
						AvailabilityZone: input.LaunchTemplateConfigs[0].Overrides[0].AvailabilityZone,
					},
				},
			},
		}}
		for _, pool := range skippedPools {
			result.Errors = append(result.Errors, &ec2.CreateFleetError{
				ErrorCode: aws.String("InsufficientInstanceCapacity"),
				LaunchTemplateAndOverrides: &ec2.LaunchTemplateAndOverridesResponse{
					Overrides: &ec2.FleetLaunchTemplateOverrides{
						InstanceType:     aws.String(pool.InstanceType),
						AvailabilityZone: aws.String(pool.Zone),
					},
				},
			})
		}
		return result, nil
	})
}

func (e *EC2API) TerminateInstancesWithContext(_ context.Context, input *ec2.TerminateInstancesInput, _ ...request.Option) (*ec2.TerminateInstancesOutput, error) {
	return e.TerminateInstancesBehavior.Invoke(input, func(input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
		var instanceStateChanges []*ec2.InstanceStateChange
		for _, id := range input.InstanceIds {
			instanceID := *id
			if _, ok := e.Instances.LoadAndDelete(instanceID); ok {
				instanceStateChanges = append(instanceStateChanges, &ec2.InstanceStateChange{
					PreviousState: &ec2.InstanceState{Name: aws.String(ec2.InstanceStateNameRunning), Code: aws.Int64(16)},
					CurrentState:  &ec2.InstanceState{Name: aws.String(ec2.InstanceStateNameShuttingDown), Code: aws.Int64(32)},
					InstanceId:    aws.String(instanceID),
				})
			}
		}
		return &ec2.TerminateInstancesOutput{TerminatingInstances: instanceStateChanges}, nil
	})
}

func (e *EC2API) CreateLaunchTemplateWithContext(_ context.Context, input *ec2.CreateLaunchTemplateInput, _ ...request.Option) (*ec2.CreateLaunchTemplateOutput, error) {
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return nil, e.NextError.Get()
	}
	e.CalledWithCreateLaunchTemplateInput.Add(input)
	launchTemplate := &ec2.LaunchTemplate{LaunchTemplateName: input.LaunchTemplateName}
	e.LaunchTemplates.Store(input.LaunchTemplateName, launchTemplate)
	return &ec2.CreateLaunchTemplateOutput{LaunchTemplate: launchTemplate}, nil
}

func (e *EC2API) CreateTagsWithContext(_ context.Context, input *ec2.CreateTagsInput, _ ...request.Option) (*ec2.CreateTagsOutput, error) {
	return e.CreateTagsBehavior.Invoke(input, func(input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
		// Update passed in instances with the passed tags
		for _, id := range input.Resources {
			raw, ok := e.Instances.Load(aws.StringValue(id))
			if !ok {
				return nil, fmt.Errorf("instance with id '%s' does not exist", aws.StringValue(id))
			}
			instance := raw.(*ec2.Instance)

			// Upsert any tags that have the same key
			newTagKeys := sets.New(lo.Map(input.Tags, func(t *ec2.Tag, _ int) string { return aws.StringValue(t.Key) })...)
			instance.Tags = lo.Filter(input.Tags, func(t *ec2.Tag, _ int) bool { return newTagKeys.Has(aws.StringValue(t.Key)) })
			instance.Tags = append(instance.Tags, input.Tags...)
		}
		return nil, nil
	})
}

func (e *EC2API) DescribeInstancesWithContext(_ context.Context, input *ec2.DescribeInstancesInput, _ ...request.Option) (*ec2.DescribeInstancesOutput, error) {
	return e.DescribeInstancesBehavior.Invoke(input, func(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
		var instances []*ec2.Instance

		// If it's a list call and no instance ids are specified
		if len(aws.StringValueSlice(input.InstanceIds)) == 0 {
			e.Instances.Range(func(k interface{}, v interface{}) bool {
				instances = append(instances, v.(*ec2.Instance))
				return true
			})
		}
		for _, instanceID := range input.InstanceIds {
			instance, _ := e.Instances.Load(*instanceID)
			if instance == nil {
				continue
			}
			instances = append(instances, instance.(*ec2.Instance))
		}
		return &ec2.DescribeInstancesOutput{
			Reservations: []*ec2.Reservation{{Instances: filterInstances(instances, input.Filters)}},
		}, nil
	})
}

func (e *EC2API) DescribeInstancesPagesWithContext(ctx context.Context, input *ec2.DescribeInstancesInput, fn func(*ec2.DescribeInstancesOutput, bool) bool, opts ...request.Option) error {
	output, err := e.DescribeInstancesWithContext(ctx, input, opts...)
	if err != nil {
		return err
	}
	fn(output, false)
	return nil
}

//nolint:gocyclo
func filterInstances(instances []*ec2.Instance, filters []*ec2.Filter) []*ec2.Instance {
	var ret []*ec2.Instance
	for _, instance := range instances {
		passesFilter := true
	OUTER:
		for _, filter := range filters {
			switch {
			case aws.StringValue(filter.Name) == "instance-state-name":
				if !sets.New(aws.StringValueSlice(filter.Values)...).Has(aws.StringValue(instance.State.Name)) {
					passesFilter = false
					break OUTER
				}
			case aws.StringValue(filter.Name) == "tag-key":
				values := sets.New(aws.StringValueSlice(filter.Values)...)
				if _, ok := lo.Find(instance.Tags, func(t *ec2.Tag) bool {
					return values.Has(aws.StringValue(t.Key))
				}); !ok {
					passesFilter = false
					break OUTER
				}
			case strings.HasPrefix(aws.StringValue(filter.Name), "tag:"):
				k := strings.TrimPrefix(aws.StringValue(filter.Name), "tag:")
				tag, ok := lo.Find(instance.Tags, func(t *ec2.Tag) bool {
					return aws.StringValue(t.Key) == k
				})
				if !ok {
					passesFilter = false
					break OUTER
				}
				switch {
				case lo.Contains(aws.StringValueSlice(filter.Values), "*"):
				case lo.Contains(aws.StringValueSlice(filter.Values), aws.StringValue(tag.Value)):
				default:
					passesFilter = false
					break OUTER
				}
			}
		}
		if passesFilter {
			ret = append(ret, instance)
		}
	}
	return ret
}

func (e *EC2API) DescribeImagesWithContext(_ context.Context, input *ec2.DescribeImagesInput, _ ...request.Option) (*ec2.DescribeImagesOutput, error) {
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return nil, e.NextError.Get()
	}
	e.CalledWithDescribeImagesInput.Add(input)
	if !e.DescribeImagesOutput.IsNil() {
		return e.DescribeImagesOutput.Clone(), nil
	}
	if aws.StringValue(input.Filters[0].Values[0]) == "invalid" {
		return &ec2.DescribeImagesOutput{}, nil
	}
	return &ec2.DescribeImagesOutput{
		Images: []*ec2.Image{
			{
				Name:         aws.String(test.RandomName()),
				ImageId:      aws.String(test.RandomName()),
				CreationDate: aws.String(time.Now().Format(time.UnixDate)),
				Architecture: aws.String("x86_64"),
			},
		},
	}, nil
}

func (e *EC2API) DescribeLaunchTemplatesWithContext(_ context.Context, input *ec2.DescribeLaunchTemplatesInput, _ ...request.Option) (*ec2.DescribeLaunchTemplatesOutput, error) {
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return nil, e.NextError.Get()
	}
	if !e.DescribeLaunchTemplatesOutput.IsNil() {
		return e.DescribeLaunchTemplatesOutput.Clone(), nil
	}
	output := &ec2.DescribeLaunchTemplatesOutput{}
	e.LaunchTemplates.Range(func(key, value interface{}) bool {
		launchTemplate := value.(*ec2.LaunchTemplate)
		if lo.Contains(aws.StringValueSlice(input.LaunchTemplateNames), aws.StringValue(launchTemplate.LaunchTemplateName)) {
			output.LaunchTemplates = append(output.LaunchTemplates, launchTemplate)
		}
		return true
	})
	if len(output.LaunchTemplates) == 0 {
		return nil, awserr.New("InvalidLaunchTemplateName.NotFoundException", "not found", nil)
	}
	return output, nil
}

func (e *EC2API) DescribeSubnetsWithContext(_ context.Context, input *ec2.DescribeSubnetsInput, _ ...request.Option) (*ec2.DescribeSubnetsOutput, error) {
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return nil, e.NextError.Get()
	}
	if !e.DescribeSubnetsOutput.IsNil() {
		describeSubnetsOutput := e.DescribeSubnetsOutput.Clone()
		describeSubnetsOutput.Subnets = FilterDescribeSubnets(describeSubnetsOutput.Subnets, input.Filters)
		return describeSubnetsOutput, nil
	}
	subnets := []*ec2.Subnet{
		{
			SubnetId:                aws.String("subnet-test1"),
			AvailabilityZone:        aws.String("test-zone-1a"),
			AvailableIpAddressCount: aws.Int64(100),
			MapPublicIpOnLaunch:     aws.Bool(false),
			Tags: []*ec2.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-subnet-1")},
				{Key: aws.String("foo"), Value: aws.String("bar")},
			},
		},
		{
			SubnetId:                aws.String("subnet-test2"),
			AvailabilityZone:        aws.String("test-zone-1b"),
			AvailableIpAddressCount: aws.Int64(100),
			MapPublicIpOnLaunch:     aws.Bool(true),
			Tags: []*ec2.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-subnet-2")},
				{Key: aws.String("foo"), Value: aws.String("bar")},
			},
		},
		{
			SubnetId:                aws.String("subnet-test3"),
			AvailabilityZone:        aws.String("test-zone-1c"),
			AvailableIpAddressCount: aws.Int64(100),
			Tags: []*ec2.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-subnet-3")},
				{Key: aws.String("TestTag")},
				{Key: aws.String("foo"), Value: aws.String("bar")},
			},
		},
	}
	if len(input.Filters) == 0 {
		return nil, fmt.Errorf("InvalidParameterValue: The filter 'null' is invalid")
	}
	return &ec2.DescribeSubnetsOutput{Subnets: FilterDescribeSubnets(subnets, input.Filters)}, nil
}

func (e *EC2API) DescribeSecurityGroupsWithContext(_ context.Context, input *ec2.DescribeSecurityGroupsInput, _ ...request.Option) (*ec2.DescribeSecurityGroupsOutput, error) {
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return nil, e.NextError.Get()
	}
	if !e.DescribeSecurityGroupsOutput.IsNil() {
		describeSecurityGroupsOutput := e.DescribeSecurityGroupsOutput.Clone()
		describeSecurityGroupsOutput.SecurityGroups = FilterDescribeSecurtyGroups(describeSecurityGroupsOutput.SecurityGroups, input.Filters)
		return e.DescribeSecurityGroupsOutput.Clone(), nil
	}
	sgs := []*ec2.SecurityGroup{
		{
			GroupId:   aws.String("sg-test1"),
			GroupName: aws.String("securityGroup-test1"),
			Tags: []*ec2.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-security-group-1")},
				{Key: aws.String("foo"), Value: aws.String("bar")},
			},
		},
		{
			GroupId:   aws.String("sg-test2"),
			GroupName: aws.String("securityGroup-test2"),
			Tags: []*ec2.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-security-group-2")},
				{Key: aws.String("foo"), Value: aws.String("bar")},
			},
		},
		{
			GroupId:   aws.String("sg-test3"),
			GroupName: aws.String("securityGroup-test3"),
			Tags: []*ec2.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-security-group-3")},
				{Key: aws.String("TestTag")},
				{Key: aws.String("foo"), Value: aws.String("bar")},
			},
		},
	}
	if len(input.Filters) == 0 {
		return nil, fmt.Errorf("InvalidParameterValue: The filter 'null' is invalid")
	}
	return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: FilterDescribeSecurtyGroups(sgs, input.Filters)}, nil
}

func (e *EC2API) DescribeAvailabilityZonesWithContext(context.Context, *ec2.DescribeAvailabilityZonesInput, ...request.Option) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return nil, e.NextError.Get()
	}
	if !e.DescribeAvailabilityZonesOutput.IsNil() {
		return e.DescribeAvailabilityZonesOutput.Clone(), nil
	}
	return &ec2.DescribeAvailabilityZonesOutput{AvailabilityZones: []*ec2.AvailabilityZone{
		{ZoneName: aws.String("test-zone-1a"), ZoneId: aws.String("testzone1a")},
		{ZoneName: aws.String("test-zone-1b"), ZoneId: aws.String("testzone1b")},
		{ZoneName: aws.String("test-zone-1c"), ZoneId: aws.String("testzone1c")},
	}}, nil
}

func (e *EC2API) DescribeInstanceTypesPagesWithContext(_ context.Context, _ *ec2.DescribeInstanceTypesInput, fn func(*ec2.DescribeInstanceTypesOutput, bool) bool, _ ...request.Option) error {
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return e.NextError.Get()
	}
	if !e.DescribeInstanceTypesOutput.IsNil() {
		fn(e.DescribeInstanceTypesOutput.Clone(), false)
		return nil
	}
	fn(defaultDescribeInstanceTypesOutput, false)
	return nil
}

func (e *EC2API) DescribeInstanceTypeOfferingsPagesWithContext(_ context.Context, _ *ec2.DescribeInstanceTypeOfferingsInput, fn func(*ec2.DescribeInstanceTypeOfferingsOutput, bool) bool, _ ...request.Option) error {
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return e.NextError.Get()
	}
	if !e.DescribeInstanceTypeOfferingsOutput.IsNil() {
		fn(e.DescribeInstanceTypeOfferingsOutput.Clone(), false)
		return nil
	}
	fn(&ec2.DescribeInstanceTypeOfferingsOutput{
		InstanceTypeOfferings: []*ec2.InstanceTypeOffering{
			{
				InstanceType: aws.String("m5.large"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("m5.large"),
				Location:     aws.String("test-zone-1b"),
			},
			{
				InstanceType: aws.String("m5.large"),
				Location:     aws.String("test-zone-1c"),
			},
			{
				InstanceType: aws.String("m5.xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("m5.xlarge"),
				Location:     aws.String("test-zone-1b"),
			},
			{
				InstanceType: aws.String("m5.2xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("m5.4xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("m5.8xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("p3.8xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("p3.8xlarge"),
				Location:     aws.String("test-zone-1b"),
			},
			{
				InstanceType: aws.String("dl1.24xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("dl1.24xlarge"),
				Location:     aws.String("test-zone-1b"),
			},
			{
				InstanceType: aws.String("g4dn.8xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("g4dn.8xlarge"),
				Location:     aws.String("test-zone-1b"),
			},
			{
				InstanceType: aws.String("t3.large"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("t3.large"),
				Location:     aws.String("test-zone-1b"),
			},
			{
				InstanceType: aws.String("inf1.2xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("inf1.6xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("trn1.2xlarge"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("c6g.large"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("m5.metal"),
				Location:     aws.String("test-zone-1a"),
			},
			{
				InstanceType: aws.String("m5.metal"),
				Location:     aws.String("test-zone-1b"),
			},
			{
				InstanceType: aws.String("m5.metal"),
				Location:     aws.String("test-zone-1c"),
			},
		},
	}, false)
	return nil
}

func (e *EC2API) DescribeSpotPriceHistoryPagesWithContext(_ aws.Context, in *ec2.DescribeSpotPriceHistoryInput, fn func(*ec2.DescribeSpotPriceHistoryOutput, bool) bool, _ ...request.Option) error {
	e.DescribeSpotPriceHistoryInput.Set(in)
	if !e.NextError.IsNil() {
		defer e.NextError.Reset()
		return e.NextError.Get()
	}
	if !e.DescribeSpotPriceHistoryOutput.IsNil() {
		fn(e.DescribeSpotPriceHistoryOutput.Clone(), false)
		return nil
	}
	// fail if the test doesn't provide specific data which causes our pricing provider to use its static price list
	return errors.New("no pricing data provided")
}
