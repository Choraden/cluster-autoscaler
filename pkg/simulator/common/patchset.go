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

package common

import "iter"

// PatchSet manages a stack of patches, allowing for fork/revert/commit operations.
// It provides a view of the data as if all patches were applied sequentially.
//
// Time Complexities:
//   - Fork(): O(1).
//   - Commit(): O(P), where P is the number of modified/deleted entries
//     in the current patch or no-op for PatchSet with a single patch.
//   - Revert(): O(P * N), where P is the number of modified/deleted entries
//     in the topmost patch or no-op for PatchSet with a single patch. Each of
//     the P keys is re-resolved against the remaining layers so that a cache
//     which was in sync stays in sync.
//   - FindValue(key): O(1) for cached keys or when the cache is in sync,
//     O(N) - otherwise.
//   - AsMap(), ValuesSlice(), Values(), Len(): O(C) when the cache is in sync,
//     where C is the actual number of key/value pairs in the flattened PatchSet.
//     Otherwise the cache needs to be rebuilt first, which requires iterating
//     through all the layers modifications and deletions - O(N * P).
//   - SetCurrent(key, value): O(1).
//   - DeleteCurrent(key): O(1).
//   - InCurrentPatch(key): O(1).
//
// Variables used in complexity analysis:
//   - N: The number of patch layers in the PatchSet.
//   - P: The number of modified/deleted entries in a single patch layer
//
// Caching:
//
// The PatchSet employs a lazy caching mechanism to speed up access to data. When a specific item is requested,
// the cache is checked first. If the item isn't cached, its effective value is computed by traversing the layers
// of patches from the most recent to the oldest, and the result is then stored in the cache. For operations that
// require the entire dataset like AsMap, if a cache is fully up-to-date, the data is served directly from the cache.
// Otherwise, the entire dataset is rebuilt by applying all patches, and this process fully populates the cache.
// Direct modifications to the current patch update the specific item in the cache immediately. Reverting the latest
// patch re-resolves the effective values of the affected items from the remaining layers, so a cache that was in
// sync remains in sync. Committing and forking does not invalidate the cache, as the effective values remain
// consistent from the perspective of read operations.
//
// Once the cache is in sync it holds exactly the effective key/value pairs of the PatchSet: it never contains
// nil (negative) entries, and a cache miss is a definitive "not found" answer. Negative entries are only
// recorded while the cache is not in sync.
type PatchSet[K comparable, V any] struct {
	// patches is a stack of individual modification layers. The base data is
	// at index 0, and subsequent modifications are layered on top.
	// PatchSet should always contain at least a single patch.
	patches []*Patch[K, V]

	// cache stores the computed effective value for keys that have been accessed.
	// A nil pointer indicates the key is effectively deleted or not present.
	// When cacheInSync is true, the cache contains exactly the effective
	// key/value pairs and no nil pointers.
	cache map[K]*V

	// cacheInSync indicates whether the cache map accurately reflects the
	// current state derived from applying all patches in the 'patches' slice.
	// When false, the cache may be stale and needs to be rebuilt or validated
	// before being fully trusted for all keys.
	cacheInSync bool
}

// NewPatchSet creates a new PatchSet, initializing it with the provided base patches.
func NewPatchSet[K comparable, V any](patches ...*Patch[K, V]) *PatchSet[K, V] {
	ps := &PatchSet[K, V]{
		patches: patches,
		cache:   make(map[K]*V),
	}
	// If no patch contains any modified entries, the effective state is empty
	// and the (empty) cache already reflects it.
	ps.cacheInSync = ps.totalKeyCount() == 0
	return ps
}

// Fork adds a new, empty patch layer to the top of the stack.
// Subsequent modifications will be recorded in this new layer.
func (p *PatchSet[K, V]) Fork() {
	p.patches = append(p.patches, NewPatch[K, V]())
}

// Commit merges the topmost patch layer into the one below it.
// If there's only one layer (or none), it's a no-op.
func (p *PatchSet[K, V]) Commit() {
	if len(p.patches) < 2 {
		return
	}

	currentPatch := p.patches[len(p.patches)-1]
	previousPatch := p.patches[len(p.patches)-2]
	mergePatchesInPlace(previousPatch, currentPatch)
	p.patches = p.patches[:len(p.patches)-1]
}

// Revert removes the topmost patch layer.
// Any modifications or deletions recorded in that layer are discarded.
func (p *PatchSet[K, V]) Revert() {
	if len(p.patches) <= 1 {
		return
	}

	currentPatch := p.patches[len(p.patches)-1]
	p.patches = p.patches[:len(p.patches)-1]

	if !p.cacheInSync {
		// The cache is only a partial view - dropping the affected
		// entries is enough, they will be lazily recomputed on access.
		for key := range currentPatch.modified {
			delete(p.cache, key)
		}
		for key := range currentPatch.deleted {
			delete(p.cache, key)
		}
		return
	}

	// Only keys modified or deleted in the reverted patch could have changed
	// their effective value. Re-resolve them from the remaining layers so the
	// cache stays in sync and no full rebuild is needed on the next read.
	for key := range currentPatch.modified {
		p.repairCacheKey(key)
	}
	for key := range currentPatch.deleted {
		p.repairCacheKey(key)
	}
}

// repairCacheKey recomputes the effective value of the key from the patch
// layers and stores it in the cache, preserving the in-sync cache invariant
// (no nil entries).
func (p *PatchSet[K, V]) repairCacheKey(key K) {
	for i := len(p.patches) - 1; i >= 0; i-- {
		patch := p.patches[i]
		if patch.IsDeleted(key) {
			break
		}
		if value, found := patch.Get(key); found {
			// Copy inside the branch so that only the found value is heap
			// allocated, rather than the loop variable on every iteration.
			valueCopy := value
			p.cache[key] = &valueCopy
			return
		}
	}
	delete(p.cache, key)
}

// FindValue searches for the effective value of a key by looking through the patches
// from top to bottom. It returns the value and true if found, or the zero value and false
// if the key is deleted or not found in any patch.
func (p *PatchSet[K, V]) FindValue(key K) (value V, found bool) {
	var zero V

	if cachedValue, cacheHit := p.cache[key]; cacheHit {
		if cachedValue == nil {
			return zero, false
		}

		return *cachedValue, true
	}

	// When the cache is in sync it holds every effective key,
	// so a cache miss is a definitive negative answer.
	if p.cacheInSync {
		return zero, false
	}

	for i := len(p.patches) - 1; i >= 0; i-- {
		patch := p.patches[i]
		if patch.IsDeleted(key) {
			break
		}

		if foundValue, ok := patch.Get(key); ok {
			// Copy inside the branch so that only the found value is heap
			// allocated, rather than the loop variable on every iteration.
			valueCopy := foundValue
			p.cache[key] = &valueCopy
			return foundValue, true
		}
	}

	// The cache is not in sync, so record a negative entry to avoid
	// repeating the patch traversal for the same key.
	p.cache[key] = nil
	return zero, false
}

// AsMap merges all patches into a single map representing the current effective state.
// It iterates through all patches from bottom to top, applying modifications and deletions.
// The cache is populated with the results during this process.
func (p *PatchSet[K, V]) AsMap() map[K]V {
	p.syncCache()

	patchSetMap := make(map[K]V, len(p.cache))
	for key, value := range p.cache {
		patchSetMap[key] = *value
	}
	return patchSetMap
}

// ValuesSlice returns a newly allocated slice with all the effective values
// of the PatchSet, in no particular order. Unlike AsMap it doesn't allocate
// an intermediate map.
func (p *PatchSet[K, V]) ValuesSlice() []V {
	p.syncCache()

	values := make([]V, 0, len(p.cache))
	for _, value := range p.cache {
		values = append(values, *value)
	}
	return values
}

// Values returns an iterator over all the effective values of the PatchSet,
// in no particular order. The PatchSet must not be modified while iterating.
func (p *PatchSet[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		p.syncCache()
		for _, value := range p.cache {
			if !yield(*value) {
				return
			}
		}
	}
}

// Len returns the number of effective key/value pairs in the PatchSet.
func (p *PatchSet[K, V]) Len() int {
	p.syncCache()
	return len(p.cache)
}

// syncCache rebuilds the cache from all the patch layers if it's not in sync.
// After it returns the cache contains exactly the effective key/value pairs.
func (p *PatchSet[K, V]) syncCache() {
	if p.cacheInSync {
		return
	}

	keysCount := p.totalKeyCount()
	if len(p.cache) == 0 {
		p.cache = make(map[K]*V, keysCount)
	} else {
		// Drop possibly partial content including negative entries.
		clear(p.cache)
	}

	// All the values are stored in a single preallocated backing slice to
	// avoid a separate heap allocation per key. Its capacity is never
	// exceeded, so pointers into it stay valid.
	values := make([]V, 0, keysCount)

	// Resolve keys from the topmost layer down, so that each effective value
	// is stored exactly once. Keys deleted in upper layers are temporarily
	// marked with nil entries to shadow values from the lower layers.
	hasDeletions := false
	for i := len(p.patches) - 1; i >= 0; i-- {
		patch := p.patches[i]
		for key, value := range patch.modified {
			if _, seen := p.cache[key]; !seen {
				values = append(values, value)
				p.cache[key] = &values[len(values)-1]
			}
		}
		for key := range patch.deleted {
			if _, seen := p.cache[key]; !seen {
				p.cache[key] = nil
				hasDeletions = true
			}
		}
	}

	if hasDeletions {
		for key, value := range p.cache {
			if value == nil {
				delete(p.cache, key)
			}
		}
	}

	p.cacheInSync = true
}

// SetCurrent adds or updates a key-value pair in the topmost patch layer.
func (p *PatchSet[K, V]) SetCurrent(key K, value V) {
	if len(p.patches) == 0 {
		p.Fork()
	}

	currentPatch := p.patches[len(p.patches)-1]
	currentPatch.Set(key, value)
	p.cache[key] = &value
}

// DeleteCurrent marks a key as deleted in the topmost patch layer.
func (p *PatchSet[K, V]) DeleteCurrent(key K) {
	if len(p.patches) == 0 {
		p.Fork()
	}

	currentPatch := p.patches[len(p.patches)-1]
	currentPatch.Delete(key)
	if p.cacheInSync {
		// An in-sync cache never holds negative entries.
		delete(p.cache, key)
	} else {
		p.cache[key] = nil
	}
}

// InCurrentPatch checks if the key is available in the topmost patch layer.
func (p *PatchSet[K, V]) InCurrentPatch(key K) bool {
	if len(p.patches) == 0 {
		return false
	}

	currentPatch := p.patches[len(p.patches)-1]
	_, found := currentPatch.Get(key)
	return found
}

// totalKeyCount calculates an approximate total number of key-value
// pairs across all patches taking the highest number of records possible
// this calculation does not consider deleted records - thus it is likely
// to be not accurate
func (p *PatchSet[K, V]) totalKeyCount() int {
	count := 0
	for _, patch := range p.patches {
		count += len(patch.modified)
	}

	return count
}

// ClonePatchSet creates a deep copy of a PatchSet object with the same patch layers
// structure, while copying keys and values using cloneKey and cloneValue functions
// provided.
//
// This function is intended for testing purposes only.
func ClonePatchSet[K comparable, V any](ps *PatchSet[K, V], cloneKey func(K) K, cloneValue func(V) V) *PatchSet[K, V] {
	if ps == nil {
		return nil
	}

	clonedPatches := make([]*Patch[K, V], 0, len(ps.patches))
	for _, patch := range ps.patches {
		clonedPatch := NewPatch[K, V]()
		for key, value := range patch.modified {
			clonedKey, clonedValue := cloneKey(key), cloneValue(value)
			clonedPatch.Set(clonedKey, clonedValue)
		}

		for key := range patch.deleted {
			clonedKey := cloneKey(key)
			clonedPatch.Delete(clonedKey)
		}

		clonedPatches = append(clonedPatches, clonedPatch)
	}

	// NewPatchSet derives the initial cache state from the cloned layers.
	return NewPatchSet(clonedPatches...)
}
