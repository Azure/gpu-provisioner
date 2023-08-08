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

package functional

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

type Pair[A, B any] struct {
	First  A
	Second B
}

type Option[T any] func(T) T

func ResolveOptions[T any](opts ...Option[T]) T {
	o := *new(T)
	for _, opt := range opts {
		if opt != nil {
			o = opt(o)
		}
	}
	return o
}

// HasAnyPrefix returns true if any of the provided prefixes match the given string s
func HasAnyPrefix(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// SplitCommaSeparatedString splits a string by commas, removes whitespace, and returns
// a slice of strings
func SplitCommaSeparatedString(value string) []string {
	var result []string
	for _, value := range strings.Split(value, ",") {
		result = append(result, strings.TrimSpace(value))
	}
	return result
}

func Unmarshal[T any](raw []byte) (*T, error) {
	t := *new(T)
	if err := yaml.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func FilterMap[K comparable, V any](m map[K]V, f func(K, V) bool) map[K]V {
	ret := map[K]V{}
	for k, v := range m {
		if f(k, v) {
			ret[k] = v
		}
	}
	return ret
}
