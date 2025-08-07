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

package utils

import (
	sdkerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
)

// IsAzureNotFoundError checks if an error is an Azure "NotFound" error
func IsAzureNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	azErr := sdkerrors.IsResponseError(err)
	return azErr != nil && azErr.ErrorCode == "NotFound"
}

// ShouldIgnoreNotFoundError returns nil if the error is a "NotFound" error, otherwise returns the original error
func ShouldIgnoreNotFoundError(err error) error {
	if IsAzureNotFoundError(err) {
		return nil
	}
	return err
}
