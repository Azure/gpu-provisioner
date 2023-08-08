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
	"regexp"
	"strconv"
)

type VMSizeType struct {
	family           string
	subfamily        *string
	cpus             string
	cpusConstrained  *string
	additiveFeatures []rune
	acceleratorType  *string
	version          string
}

var (
	// https://docs.microsoft.com/en-us/azure/virtual-machines/vm-naming-conventions
	// [Family] + [Sub-family]* + [# of vCPUs] + [Constrained vCPUs]* + [Additive Features] + [Accelerator Type]* + [Version] + [_Promo]
	// ((?:re)?) pattern is used to capture segment of interest or empty string (for optional segment)
	// TODO: capture _Promo, what is 'r'?
	skuSizeScheme = regexp.MustCompile(
		`^([A-Z])([A-Z]?)([0-9]+)-?((?:[0-9]+)?)((?:[abcdilmtspPr]+|NP)?)_?((?:[A-Z][0-9]+)?)_?((?:[vV][1-9])?)(_Promo)?$`,
	)
)

func getVMSize(vmSizeName string) (*VMSizeType, error) {
	vmsize := VMSizeType{}

	parts := skuSizeScheme.FindStringSubmatch(vmSizeName)
	if parts == nil || len(parts) < 8 {
		return nil, fmt.Errorf("could not parse VM size %s", vmSizeName)
	}

	// [Family]
	vmsize.family = parts[1]

	// [Sub-family]*
	if len(parts[2]) > 0 {
		vmsize.subfamily = &parts[2]
	}

	// [# of vCPUs]
	vmsize.cpus = parts[3]

	// [Constrained vCPUs]*
	if len(parts[4]) > 0 {
		_, err := strconv.Atoi(parts[4]) // just checking
		if err != nil {
			return nil, fmt.Errorf("converting constrained CPUs, %w", err)
		}
		vmsize.cpusConstrained = &parts[4]
	}

	// [Additive Features]
	// TODO: handle "NP"
	vmsize.additiveFeatures = []rune(parts[5])

	// [Accelerator Type]*
	if len(parts[6]) > 0 {
		vmsize.acceleratorType = &parts[6]
	}

	// [Version]
	vmsize.version = parts[7]

	return &vmsize, nil
}

// e.g. ....: family + subfamily + additive features + version
func (vmsize *VMSizeType) getSeries() string {
	subfamily := ""
	if vmsize.subfamily != nil {
		subfamily = *vmsize.subfamily
	}
	additiveFeatures := string(vmsize.additiveFeatures)
	version := ""
	if len(vmsize.version) > 0 {
		version = "_" + vmsize.version
	}
	return vmsize.family + subfamily + additiveFeatures + version
}

func (vmsize *VMSizeType) String() string {
	subfamily := ""
	if vmsize.subfamily != nil {
		subfamily = *vmsize.subfamily
	}
	cpusConstrained := ""
	if vmsize.cpusConstrained != nil {
		subfamily = *vmsize.cpusConstrained
	}
	accType := ""
	if vmsize.acceleratorType != nil {
		subfamily = *vmsize.acceleratorType
	}

	return fmt.Sprintf("fam=%s, subfam=%s, cpu=%s, cpuc=%s, feat=%s, acc=%s, ver=%s; series=%s",
		vmsize.family, subfamily, vmsize.cpus, cpusConstrained, string(vmsize.additiveFeatures), accType, vmsize.version, vmsize.getSeries())
}
