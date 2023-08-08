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

package rebalancerecommendation

import (
	"encoding/json"
	"fmt"

	"github.com/aws/karpenter/pkg/controllers/interruption/messages"
)

type Parser struct{}

func (p Parser) Parse(raw string) (messages.Message, error) {
	msg := Message{}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return nil, fmt.Errorf("unmarhsalling the message as EC2InstanceRebalanceRecommendation, %w", err)
	}
	return msg, nil
}

func (p Parser) Version() string {
	return "0"
}

func (p Parser) Source() string {
	return "aws.ec2"
}

func (p Parser) DetailType() string {
	return "EC2 Instance Rebalance Recommendation"
}
