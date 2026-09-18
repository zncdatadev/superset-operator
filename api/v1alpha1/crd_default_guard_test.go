/*
Copyright 2024 zncdatadev.

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

package v1alpha1

import (
	"path/filepath"
	"testing"

	"github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/testutil"
)

func TestGeneratedCRDs_HaveNoInheritedConfigDefaults(t *testing.T) {
	t.Parallel()

	crdGlob := filepath.Join("..", "..", "config", "crd", "bases", "*.yaml")
	gomega.NewWithT(t).Expect(crdGlob).To(testutil.HaveNoInheritedConfigDefaults())
}
