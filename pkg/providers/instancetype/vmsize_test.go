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

package instancetype

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVMSizeParsing(t *testing.T) {
	v, c, t4, theNumber2 := "V", "C", "T4", "2"
	a := assert.New(t)
	tc := []struct {
		size       string
		expectedVM VMSizeType
	}{
		{
			size: "NV16as_v4",
			expectedVM: VMSizeType{
				family:           "N",
				subfamily:        &v,
				cpus:             "16",
				cpusConstrained:  nil,
				additiveFeatures: []rune{'a', 's'},
				acceleratorType:  nil,
				version:          "v4",
			},
		},
		{
			size: "M16ms_v2",
			expectedVM: VMSizeType{
				family:           "M",
				subfamily:        nil,
				cpus:             "16",
				cpusConstrained:  nil,
				additiveFeatures: []rune{'m', 's'},
				acceleratorType:  nil,
				version:          "v2",
			},
		},
		{
			size: "NC4as_T4_v3",
			expectedVM: VMSizeType{
				family:           "N",
				subfamily:        &c,
				cpus:             "4",
				cpusConstrained:  nil,
				additiveFeatures: []rune{'a', 's'},
				acceleratorType:  &t4,
				version:          "v3",
			},
		},
		{
			size: "M8-2ms_v2",
			expectedVM: VMSizeType{
				family:           "M",
				subfamily:        nil,
				cpus:             "8",
				cpusConstrained:  &theNumber2,
				additiveFeatures: []rune{'m', 's'},
				acceleratorType:  nil,
				version:          "v2",
			},
		},
		{
			size: "A4_v2",
			expectedVM: VMSizeType{
				family:           "A",
				subfamily:        nil,
				cpus:             "4",
				cpusConstrained:  nil,
				additiveFeatures: []rune{},
				acceleratorType:  nil,
				version:          "v2",
			},
		},
	}

	for _, c := range tc {
		s, err := getVMSize(c.size)
		if err != nil {
			t.Fatalf(`Parsing %s, %v`, c.size, err)
		}
		fmt.Println(c.size)
		a.Equal(c.expectedVM.family, s.family)
		a.Equal(c.expectedVM.subfamily, s.subfamily)
		a.Equal(c.expectedVM.cpus, s.cpus)
		a.Equal(c.expectedVM.acceleratorType, s.acceleratorType)
		a.Equal(c.expectedVM.cpusConstrained, s.cpusConstrained)
		a.Equal(c.expectedVM.version, s.version)
		a.Equal(c.expectedVM.additiveFeatures, s.additiveFeatures)
	}
}
