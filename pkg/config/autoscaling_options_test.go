/*
Copyright 2026 The Kubernetes Authors.

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

package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	gce_localssdsize "sigs.k8s.io/cluster-autoscaler/pkg/cloudprovider/gce/localssdsize"
)

func TestDefaultAutoscalingOptions(t *testing.T) {
	cmpOpts := cmp.AllowUnexported(gce_localssdsize.SimpleLocalSSDProvider{})

	t.Run("modifiers are applied in order", func(t *testing.T) {
		got := DefaultAutoscalingOptions(
			func(o *AutoscalingOptions) {
				o.MaxNodesTotal = 1
				o.ScanInterval = time.Second
			},
			func(o *AutoscalingOptions) {
				o.MaxNodesTotal = 2
				o.NodeGroupDefaults.ScaleDownUnneededTime = time.Minute
			},
		)
		want := DefaultAutoscalingOptions()
		want.MaxNodesTotal = 2
		want.ScanInterval = time.Second
		want.NodeGroupDefaults.ScaleDownUnneededTime = time.Minute
		if diff := cmp.Diff(want, got, cmpOpts); diff != "" {
			t.Errorf("DefaultAutoscalingOptions() with modifiers unexpected result (-want +got):\n%s", diff)
		}
	})

	t.Run("returned values are independent", func(t *testing.T) {
		a := DefaultAutoscalingOptions()
		b := DefaultAutoscalingOptions()
		a.MaxNodesTotal = 42
		a.NodeGroupDefaults.MaxNodeProvisionTime = time.Second
		assert.Equal(t, 0, b.MaxNodesTotal)
		assert.Equal(t, 15*time.Minute, b.NodeGroupDefaults.MaxNodeProvisionTime)
	})
}
