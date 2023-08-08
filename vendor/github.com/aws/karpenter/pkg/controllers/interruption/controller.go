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
	"context"
	"fmt"
	"time"

	sqsapi "github.com/aws/aws-sdk-go/service/sqs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aws/karpenter-core/pkg/utils/pretty"
	"github.com/aws/karpenter/pkg/apis/settings"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	"github.com/aws/karpenter/pkg/cache"
	interruptionevents "github.com/aws/karpenter/pkg/controllers/interruption/events"
	"github.com/aws/karpenter/pkg/controllers/interruption/messages"
	"github.com/aws/karpenter/pkg/controllers/interruption/messages/statechange"
	"github.com/aws/karpenter/pkg/utils"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/events"
	"github.com/aws/karpenter-core/pkg/metrics"
	corecontroller "github.com/aws/karpenter-core/pkg/operator/controller"
)

type Action string

const (
	CordonAndDrain Action = "CordonAndDrain"
	NoAction       Action = "NoAction"
)

// Controller is an AWS interruption controller.
// It continually polls an SQS queue for events from aws.ec2 and aws.health that
// trigger node health events or node spot interruption/rebalance events.
type Controller struct {
	kubeClient                client.Client
	clk                       clock.Clock
	recorder                  events.Recorder
	sqsProvider               *SQSProvider
	unavailableOfferingsCache *cache.UnavailableOfferings
	parser                    *EventParser
	cm                        *pretty.ChangeMonitor
}

func NewController(kubeClient client.Client, clk clock.Clock, recorder events.Recorder,
	sqsProvider *SQSProvider, unavailableOfferingsCache *cache.UnavailableOfferings) *Controller {

	return &Controller{
		kubeClient:                kubeClient,
		clk:                       clk,
		recorder:                  recorder,
		sqsProvider:               sqsProvider,
		unavailableOfferingsCache: unavailableOfferingsCache,
		parser:                    NewEventParser(DefaultParsers...),
		cm:                        pretty.NewChangeMonitor(),
	}
}

func (c *Controller) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("queue", settings.FromContext(ctx).InterruptionQueueName))
	if c.cm.HasChanged(settings.FromContext(ctx).InterruptionQueueName, nil) {
		logging.FromContext(ctx).Debugf("watching interruption queue")
	}
	sqsMessages, err := c.sqsProvider.GetSQSMessages(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting messages from queue, %w", err)
	}
	if len(sqsMessages) == 0 {
		return reconcile.Result{}, nil
	}
	machineInstanceIDMap, err := c.makeMachineInstanceIDMap(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("making machine instance id map, %w", err)
	}
	nodeInstanceIDMap, err := c.makeNodeInstanceIDMap(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("making node instance id map, %w", err)
	}
	errs := make([]error, len(sqsMessages))
	workqueue.ParallelizeUntil(ctx, 10, len(sqsMessages), func(i int) {
		msg, e := c.parseMessage(sqsMessages[i])
		if e != nil {
			// If we fail to parse, then we should delete the message but still log the error
			logging.FromContext(ctx).Errorf("parsing message, %v", e)
			errs[i] = c.deleteMessage(ctx, sqsMessages[i])
			return
		}
		if e = c.handleMessage(ctx, machineInstanceIDMap, nodeInstanceIDMap, msg); e != nil {
			errs[i] = fmt.Errorf("handling message, %w", e)
			return
		}
		errs[i] = c.deleteMessage(ctx, sqsMessages[i])
	})
	return reconcile.Result{}, multierr.Combine(errs...)
}

func (c *Controller) Name() string {
	return "interruption"
}

func (c *Controller) Builder(_ context.Context, m manager.Manager) corecontroller.Builder {
	return corecontroller.NewSingletonManagedBy(m)
}

// parseMessage parses the passed SQS message into an internal Message interface
func (c *Controller) parseMessage(raw *sqsapi.Message) (messages.Message, error) {
	// No message to parse in this case
	if raw == nil || raw.Body == nil {
		return nil, fmt.Errorf("message or message body is nil")
	}
	msg, err := c.parser.Parse(*raw.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing sqs message, %w", err)
	}
	return msg, nil
}

// handleMessage takes an action against every node involved in the message that is owned by a Provisioner
func (c *Controller) handleMessage(ctx context.Context, machineInstanceIDMap map[string]*v1alpha5.Machine,
	nodeInstanceIDMap map[string]*v1.Node, msg messages.Message) (err error) {

	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("messageKind", msg.Kind()))
	receivedMessages.WithLabelValues(string(msg.Kind())).Inc()

	if msg.Kind() == messages.NoOpKind {
		return nil
	}
	for _, instanceID := range msg.EC2InstanceIDs() {
		machine, ok := machineInstanceIDMap[instanceID]
		if !ok {
			continue
		}
		node := nodeInstanceIDMap[instanceID]
		if e := c.handleMachine(ctx, msg, machine, node); e != nil {
			err = multierr.Append(err, e)
		}
	}
	messageLatency.Observe(time.Since(msg.StartTime()).Seconds())
	if err != nil {
		return fmt.Errorf("acting on machines, %w", err)
	}
	return nil
}

// deleteMessage removes the passed SQS message from the queue and fires a metric for the deletion
func (c *Controller) deleteMessage(ctx context.Context, msg *sqsapi.Message) error {
	if err := c.sqsProvider.DeleteSQSMessage(ctx, msg); err != nil {
		return fmt.Errorf("deleting sqs message, %w", err)
	}
	deletedMessages.Inc()
	return nil
}

// handleMachine retrieves the action for the message and then performs the appropriate action against the node
func (c *Controller) handleMachine(ctx context.Context, msg messages.Message, machine *v1alpha5.Machine, node *v1.Node) error {
	action := actionForMessage(msg)
	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("machine", machine.Name))
	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("action", string(action)))
	if node != nil {
		ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("node", node.Name))
	}

	// Record metric and event for this action
	c.notifyForMessage(msg, machine, node)
	actionsPerformed.WithLabelValues(string(action)).Inc()

	// Mark the offering as unavailable in the ICE cache since we got a spot interruption warning
	if msg.Kind() == messages.SpotInterruptionKind {
		zone := machine.Labels[v1.LabelTopologyZone]
		instanceType := machine.Labels[v1.LabelInstanceTypeStable]
		if zone != "" && instanceType != "" {
			c.unavailableOfferingsCache.MarkUnavailable(ctx, string(msg.Kind()), instanceType, zone, v1alpha1.CapacityTypeSpot)
		}
	}
	if action != NoAction {
		return c.deleteMachine(ctx, machine, node)
	}
	return nil
}

// deleteMachine removes the machine from the api-server
func (c *Controller) deleteMachine(ctx context.Context, machine *v1alpha5.Machine, node *v1.Node) error {
	if !machine.DeletionTimestamp.IsZero() {
		return nil
	}
	if err := c.kubeClient.Delete(ctx, machine); err != nil {
		return client.IgnoreNotFound(fmt.Errorf("deleting the node on interruption message, %w", err))
	}
	logging.FromContext(ctx).Infof("initiating delete for machine from interruption message")
	c.recorder.Publish(interruptionevents.TerminatingOnInterruption(node, machine)...)
	metrics.MachinesTerminatedCounter.With(prometheus.Labels{
		metrics.ReasonLabel:      terminationReasonLabel,
		metrics.ProvisionerLabel: machine.Labels[v1alpha5.ProvisionerNameLabelKey],
	}).Inc()
	return nil
}

// notifyForMessage publishes the relevant alert based on the message kind
func (c *Controller) notifyForMessage(msg messages.Message, m *v1alpha5.Machine, n *v1.Node) {
	switch msg.Kind() {
	case messages.RebalanceRecommendationKind:
		c.recorder.Publish(interruptionevents.RebalanceRecommendation(n, m)...)

	case messages.ScheduledChangeKind:
		c.recorder.Publish(interruptionevents.Unhealthy(n, m)...)

	case messages.SpotInterruptionKind:
		c.recorder.Publish(interruptionevents.SpotInterrupted(n, m)...)

	case messages.StateChangeKind:
		typed := msg.(statechange.Message)
		if lo.Contains([]string{"stopping", "stopped"}, typed.Detail.State) {
			c.recorder.Publish(interruptionevents.Stopping(n, m)...)
		} else {
			c.recorder.Publish(interruptionevents.Terminating(n, m)...)
		}

	default:
	}
}

// makeMachineInstanceIDMap builds a map between the instance id that is stored in the
// machine .status.providerID and the machine
func (c *Controller) makeMachineInstanceIDMap(ctx context.Context) (map[string]*v1alpha5.Machine, error) {
	m := map[string]*v1alpha5.Machine{}
	machineList := &v1alpha5.MachineList{}
	if err := c.kubeClient.List(ctx, machineList); err != nil {
		return nil, fmt.Errorf("listing machines, %w", err)
	}
	for i := range machineList.Items {
		if machineList.Items[i].Status.ProviderID == "" {
			continue
		}
		id, err := utils.ParseInstanceID(machineList.Items[i].Status.ProviderID)
		if err != nil || id == "" {
			continue
		}
		m[id] = &machineList.Items[i]
	}
	return m, nil
}

// makeNodeInstanceIDMap builds a map between the instance id that is stored in the
// node .spec.providerID and the node
func (c *Controller) makeNodeInstanceIDMap(ctx context.Context) (map[string]*v1.Node, error) {
	m := map[string]*v1.Node{}
	nodeList := &v1.NodeList{}
	if err := c.kubeClient.List(ctx, nodeList); err != nil {
		return nil, fmt.Errorf("listing nodes, %w", err)
	}
	for i := range nodeList.Items {
		if nodeList.Items[i].Spec.ProviderID == "" {
			continue
		}
		id, err := utils.ParseInstanceID(nodeList.Items[i].Spec.ProviderID)
		if err != nil || id == "" {
			continue
		}
		m[id] = &nodeList.Items[i]
	}
	return m, nil
}

func actionForMessage(msg messages.Message) Action {
	switch msg.Kind() {
	case messages.ScheduledChangeKind, messages.SpotInterruptionKind, messages.StateChangeKind:
		return CordonAndDrain
	default:
		return NoAction
	}
}
