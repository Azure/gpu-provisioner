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
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/mitchellh/hashstructure/v2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
)

type SSMAPI struct {
	ssmiface.SSMAPI
	Parameters         map[string]string
	GetParameterOutput *ssm.GetParameterOutput
	WantErr            error
}

func (a SSMAPI) GetParameterWithContext(_ context.Context, input *ssm.GetParameterInput, _ ...request.Option) (*ssm.GetParameterOutput, error) {
	if a.WantErr != nil {
		return nil, a.WantErr
	}
	if len(a.Parameters) > 0 {
		if amiID, ok := a.Parameters[*input.Name]; ok {
			return &ssm.GetParameterOutput{
				Parameter: &ssm.Parameter{Value: aws.String(amiID)},
			}, nil
		}
		return nil, awserr.New(ssm.ErrCodeParameterNotFound, fmt.Sprintf("%s couldn't be found", *input.Name), nil)
	}
	hc, _ := hashstructure.Hash(input.Name, hashstructure.FormatV2, nil)
	if a.GetParameterOutput != nil {
		return a.GetParameterOutput, nil
	}

	return &ssm.GetParameterOutput{
		Parameter: &ssm.Parameter{Value: aws.String(fmt.Sprintf("test-ami-id-%x", hc))},
	}, nil
}

func (a *SSMAPI) Reset() {
	a.GetParameterOutput = nil
	a.Parameters = nil
	a.WantErr = nil
}
