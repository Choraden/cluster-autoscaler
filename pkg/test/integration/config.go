/*
Copyright 2025 The Kubernetes Authors.

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

package integration

import (
	"time"

	"sigs.k8s.io/cluster-autoscaler/pkg/config"
	"sigs.k8s.io/cluster-autoscaler/pkg/estimator"
	"sigs.k8s.io/cluster-autoscaler/pkg/utils/units"
)

// DefaultAutoscalingOptions provides the baseline configuration for all tests.
// It starts from the canonical production defaults and applies test-accelerated overrides.
var DefaultAutoscalingOptions = config.DefaultAutoscalingOptions(func(o *config.AutoscalingOptions) {
	o.NodeGroupDefaults.ScaleDownUnneededTime = time.Second
	o.NodeGroupDefaults.ScaleDownUnreadyTime = time.Minute
	o.NodeGroupDefaults.ScaleDownUtilizationThreshold = 0.5
	o.NodeGroupDefaults.MaxNodeProvisionTime = 10 * time.Second
	o.MaxTotalUnreadyPercentage = 1
	o.OkTotalUnreadyCount = 100000
	o.EstimatorName = estimator.BinpackingEstimatorName
	o.EnforceNodeGroupMinSize = true
	o.ScaleDownSimulationTimeout = 24 * time.Hour
	o.ScaleDownDelayAfterAdd = 0
	o.ScaleDownDelayAfterDelete = 0
	o.ScaleDownDelayAfterFailure = 0
	o.MaxScaleDownParallelism = 10
	o.MaxDrainParallelism = 1
	o.ScaleDownDelayTypeLocal = true
	o.ScaleDownEnabled = true
	o.MaxNodesTotal = 10000
	o.MaxCoresTotal = 100000              // WARN: This setting isn't actually used by the fake CloudProvider.GetResourceLimiter(), there's a separate config there.
	o.MaxMemoryTotal = 100000 * units.GiB // WARN: This setting isn't actually used by the fake CloudProvider.GetResourceLimiter(), there's a separate config there.
	o.ExpanderNames = "least-waste"
	o.ScaleUpFromZero = true
	o.FrequentLoopsEnabled = true
	o.ClusterName = "cluster-test"
	o.MaxBinpackingTime = 10 * time.Second
	o.PredicateParallelism = 1
	// Keep synctest virtual-clock behavior unaffected: no sleep between tainting and deleting a node.
	o.NodeDeleteDelayAfterTaint = 0
	o.WriteStatusConfigMap = false
	// The in-memory test environment does not provide CSINode objects for all nodes.
	o.CSINodeAwareSchedulingEnabled = false
})

// TestConfig is the "blueprint" for a test. It defines the entire
// initial state of the world before the test runs.
type TestConfig struct {
	// BaseOptions can be set to DefaultAutoscalingOptions or a custom base.
	BaseOptions *config.AutoscalingOptions
	// OptionsOverrides allows adding options overrides.
	OptionsOverrides []AutoscalingOptionOverride
}

// NewTestConfig creates a test config pre-populated with DefaultAutoscalingOptions.
func NewTestConfig() *TestConfig {
	return &TestConfig{
		BaseOptions:      &DefaultAutoscalingOptions,
		OptionsOverrides: []AutoscalingOptionOverride{},
	}
}

// AutoscalingOptionOverride is a function that modifies an AutoscalingOptions object.
type AutoscalingOptionOverride = config.AutoscalingOptionModifier

// WithOverrides allows adding options overrides to the config.
func (c *TestConfig) WithOverrides(overrides ...AutoscalingOptionOverride) *TestConfig {
	c.OptionsOverrides = append(c.OptionsOverrides, overrides...)
	return c
}

// ResolveOptions merges the base options with all registered overrides.
func (c *TestConfig) ResolveOptions() config.AutoscalingOptions {
	var opts config.AutoscalingOptions
	if c.BaseOptions != nil {
		opts = *c.BaseOptions
	} else {
		opts = DefaultAutoscalingOptions
	}

	for _, override := range c.OptionsOverrides {
		override(&opts)
	}
	return opts
}

// WithCloudProviderName sets the cloud provider name.
func WithCloudProviderName(name string) AutoscalingOptionOverride {
	return func(o *config.AutoscalingOptions) {
		o.CloudProviderName = name
	}
}

// WithScaleDownUnneededTime sets the scale down unneeded time option.
func WithScaleDownUnneededTime(d time.Duration) AutoscalingOptionOverride {
	return func(o *config.AutoscalingOptions) {
		o.NodeGroupDefaults.ScaleDownUnneededTime = d
	}
}

// WithProvisioningRequestEnabled enables ProvisioningRequest support.
func WithProvisioningRequestEnabled() AutoscalingOptionOverride {
	return func(o *config.AutoscalingOptions) {
		o.ProvisioningRequestEnabled = true
	}
}
