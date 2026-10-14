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

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"

	"k8s.io/utils/ptr"
)

func TestPatchSetAsMap(t *testing.T) {
	tests := map[string]struct {
		patchLayers []map[string]*int
		wantMap     map[string]int
	}{
		"EmptyPatchSet": {
			patchLayers: []map[string]*int{},
			wantMap:     map[string]int{},
		},
		"SingleLayerNoDeletions": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}},
			wantMap:     map[string]int{"a": 1, "b": 2},
		},
		"SingleLayerOnlyDeletions": {
			patchLayers: []map[string]*int{{"a": nil, "b": nil}},
			wantMap:     map[string]int{},
		},
		"SingleLayerModificationsAndDeletions": {
			patchLayers: []map[string]*int{
				{"a": ptr.To(1), "b": nil, "c": ptr.To(3)}},
			wantMap: map[string]int{"a": 1, "c": 3},
		},
		"MultipleLayersNoDeletionsOverwrite": {
			patchLayers: []map[string]*int{
				{"a": ptr.To(1), "b": ptr.To(2)},
				{"b": ptr.To(22), "c": ptr.To(3)}},
			wantMap: map[string]int{"a": 1, "b": 22, "c": 3},
		},
		"MultipleLayersWithDeletions": {
			patchLayers: []map[string]*int{
				{"a": ptr.To(1), "b": nil, "x": ptr.To(100)},
				{"c": ptr.To(3), "x": ptr.To(101), "a": nil}},
			wantMap: map[string]int{"c": 3, "x": 101},
		},
		"MultipleLayersDeletionInLowerLayer": {
			patchLayers: []map[string]*int{
				{"a": nil, "b": ptr.To(2)},
				{"c": ptr.To(3)}},
			wantMap: map[string]int{"b": 2, "c": 3},
		},
		"MultipleLayersAddAfterDelete": {
			patchLayers: []map[string]*int{
				{"a": nil},
				{"a": ptr.To(11), "b": ptr.To(2)}},
			wantMap: map[string]int{"a": 11, "b": 2},
		},
		"MultipleLayersDeletePreviouslyAdded": {
			patchLayers: []map[string]*int{
				{"a": ptr.To(1), "b": ptr.To(2)},
				{"c": ptr.To(3), "b": nil}},
			wantMap: map[string]int{"a": 1, "c": 3},
		},
		"AllInOne": {
			patchLayers: []map[string]*int{
				{"k1": ptr.To(1), "k2": ptr.To(2)},
				{"k2": ptr.To(22), "k3": ptr.To(3), "k1": nil},
				{"k1": ptr.To(111), "k3": ptr.To(33), "k4": ptr.To(4), "deleted": nil},
			},
			wantMap: map[string]int{"k2": 22, "k3": 33, "k4": 4, "k1": 111},
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			patchset := buildTestPatchSet(t, test.patchLayers)
			mergedMap := patchset.AsMap()
			if !maps.Equal(mergedMap, test.wantMap) {
				t.Errorf("AsMap() result mismatch: got %v, want %v", mergedMap, test.wantMap)
			}
		})
	}
}

func TestPatchSetCommit(t *testing.T) {
	tests := map[string]struct {
		patchLayers []map[string]*int
		wantMap     map[string]int // Expected map after each commit
	}{
		"CommitEmptyPatchSet": {
			patchLayers: []map[string]*int{},
			wantMap:     map[string]int{},
		},
		"CommitSingleLayer": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}},
			wantMap:     map[string]int{"a": 1},
		},
		"CommitTwoLayersNoOverlap": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"b": ptr.To(2)}},
			wantMap:     map[string]int{"a": 1, "b": 2},
		},
		"CommitTwoLayersWithOverwrite": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"a": ptr.To(2)}},
			wantMap:     map[string]int{"a": 2},
		},
		"CommitTwoLayersWithDeleteInTopLayer": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(5)}, {"c": ptr.To(3), "a": nil}},
			wantMap:     map[string]int{"b": 5, "c": 3},
		},
		"CommitTwoLayersWithDeleteInBottomLayer": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": nil}, {"a": ptr.To(11), "c": ptr.To(3)}},
			wantMap:     map[string]int{"a": 11, "c": 3},
		},
		"CommitThreeLayers": {
			patchLayers: []map[string]*int{
				{"a": ptr.To(1), "b": ptr.To(2), "z": ptr.To(100)},
				{"b": ptr.To(22), "c": ptr.To(3), "a": nil},
				{"c": ptr.To(33), "d": ptr.To(4), "a": ptr.To(111), "b": nil, "deleted": nil},
			},
			wantMap: map[string]int{"z": 100, "c": 33, "d": 4, "a": 111},
		},
		"CommitMultipleLayersDeleteAndAddBack": {
			patchLayers: []map[string]*int{
				{"x": ptr.To(10)},
				{"x": nil},
				{"x": ptr.To(30)},
				{"y": ptr.To(40), "x": nil}},
			wantMap: map[string]int{"y": 40},
		},
		"CommitEmptyLayersBetweenDataLayers": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {}, {"b": ptr.To(2)}, {}, {"c": ptr.To(3)}},
			wantMap:     map[string]int{"a": 1, "b": 2, "c": 3},
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			ps := buildTestPatchSet(t, tc.patchLayers)
			initialNumPatches := len(ps.patches)

			if currentMap := ps.AsMap(); !maps.Equal(currentMap, tc.wantMap) {
				t.Errorf("AsMap() before any commits mismatch: got %v, want %v", currentMap, tc.wantMap)
			}

			for i := 0; i < initialNumPatches-1; i++ {
				expectedPatchesAfterCommit := len(ps.patches) - 1
				ps.Commit()
				if len(ps.patches) != expectedPatchesAfterCommit {
					t.Errorf("After commit #%d, expected %d patches, got %d", i+1, expectedPatchesAfterCommit, len(ps.patches))
				}
				if currentMap := ps.AsMap(); !maps.Equal(currentMap, tc.wantMap) {
					t.Errorf("AsMap() after commit #%d mismatch: got %v, want %v", i+1, currentMap, tc.wantMap)
				}
			}

			if initialNumPatches > 0 && len(ps.patches) != 1 {
				t.Errorf("Expected 1 patch after all commits, got %d", len(ps.patches))
			} else if initialNumPatches == 0 && len(ps.patches) != 0 {
				t.Errorf("Expected 0 patches after all commits, got %d", len(ps.patches))
			}
		})
	}
}

func TestPatchSetRevert(t *testing.T) {
	tests := map[string]struct {
		patchLayers         []map[string]*int
		wantInitialMap      map[string]int
		wantMapsAfterRevert []map[string]int
	}{
		"RevertEmptyPatchSet": {
			patchLayers:         []map[string]*int{},
			wantInitialMap:      map[string]int{},
			wantMapsAfterRevert: []map[string]int{{}},
		},
		"RevertSingleLayer": {
			patchLayers:         []map[string]*int{{"a": ptr.To(1)}},
			wantInitialMap:      map[string]int{"a": 1},
			wantMapsAfterRevert: []map[string]int{{"a": 1}},
		},
		"RevertTwoLayersNoOverlap": {
			patchLayers:         []map[string]*int{{"a": ptr.To(1)}, {"b": ptr.To(2)}},
			wantInitialMap:      map[string]int{"a": 1, "b": 2},
			wantMapsAfterRevert: []map[string]int{{"a": 1}, {"a": 1}},
		},
		"RevertTwoLayersWithOverwrite": {
			patchLayers:         []map[string]*int{{"a": ptr.To(1)}, {"a": ptr.To(2)}},
			wantInitialMap:      map[string]int{"a": 2},
			wantMapsAfterRevert: []map[string]int{{"a": 1}, {"a": 1}},
		},
		"RevertTwoLayersWithDeleteInTopLayer": {
			patchLayers:         []map[string]*int{{"a": ptr.To(1), "b": ptr.To(5)}, {"c": ptr.To(3), "a": nil}},
			wantInitialMap:      map[string]int{"b": 5, "c": 3},
			wantMapsAfterRevert: []map[string]int{{"a": 1, "b": 5}, {"a": 1, "b": 5}},
		},
		"RevertThreeLayers": {
			patchLayers: []map[string]*int{
				{"a": ptr.To(1), "b": ptr.To(2), "z": ptr.To(100)},
				{"b": ptr.To(22), "c": ptr.To(3), "a": nil},
				{"c": ptr.To(33), "d": ptr.To(4), "a": ptr.To(111), "b": nil, "deleted": nil},
			},
			wantInitialMap: map[string]int{"z": 100, "c": 33, "d": 4, "a": 111},
			wantMapsAfterRevert: []map[string]int{
				{"a": 1, "b": 2, "z": 100},
				{"a": 1, "b": 2, "z": 100},
				{"z": 100, "b": 22, "c": 3},
			},
		},
		"RevertMultipleLayersDeleteAndAddBack": {
			patchLayers:    []map[string]*int{{"x": ptr.To(10)}, {"x": nil}, {"x": ptr.To(30)}, {"y": ptr.To(40), "x": nil}},
			wantInitialMap: map[string]int{"y": 40},
			wantMapsAfterRevert: []map[string]int{
				{"x": 10},
				{"x": 10},
				{},
				{"x": 30},
			},
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			ps := buildTestPatchSet(t, tc.patchLayers)
			patchesNumber := len(ps.patches)

			if currentMap := ps.AsMap(); !maps.Equal(currentMap, tc.wantInitialMap) {
				t.Errorf("AsMap() before any reverts mismatch: got %v, want %v", currentMap, tc.wantInitialMap)
			}

			for i := 0; i <= patchesNumber; i++ {
				wantPatchesAfterRevert := len(ps.patches) - 1
				ps.Revert()
				if len(ps.patches) != wantPatchesAfterRevert && len(ps.patches) > 1 {
					t.Errorf("After revert #%d, expected %d patches, got %d", i+1, wantPatchesAfterRevert, len(ps.patches))
				}

				currentMap := ps.AsMap()
				wantMapIndex := patchesNumber - i - 1
				if wantMapIndex < 0 {
					wantMapIndex = 0
				}
				wantMapAfterRevert := tc.wantMapsAfterRevert[wantMapIndex]
				if !maps.Equal(currentMap, wantMapAfterRevert) {
					t.Errorf("AsMap() after revert #%d mismatch: got %v, want %v", i+1, currentMap, wantMapAfterRevert)
				}
			}

			if patchesNumber >= 1 && len(ps.patches) != 1 {
				t.Errorf("Expected 1 patch after all reverts, got %d", len(ps.patches))
			} else if patchesNumber == 0 && len(ps.patches) != 0 {
				t.Errorf("Expected 0 patches after all reverts, got %d", len(ps.patches))
			}
		})
	}
}

func TestPatchSetForkRevert(t *testing.T) {
	// 1. Initialize empty patchset
	ps := NewPatchSet(NewPatch[string, int]())
	if initialMap := ps.AsMap(); len(initialMap) != 0 {
		t.Fatalf("Initial AsMap() got %v, want empty", initialMap)
	}

	// 2. Call Fork
	ps.Fork()
	if len(ps.patches) != 2 {
		t.Fatalf("Expected 2 patches after Fork(), got %d", len(ps.patches))
	}

	// 3. Perform some mutation operations on the new layer
	ps.SetCurrent("a", 1)
	ps.SetCurrent("b", 2)
	ps.DeleteCurrent("a")

	// 4. Call Revert
	ps.Revert()
	if len(ps.patches) != 1 {
		t.Fatalf("Expected 1 patch after Revert(), got %d", len(ps.patches))
	}

	// 5. Compare state to the empty map
	if finalMap := ps.AsMap(); len(finalMap) != 0 {
		t.Errorf("AsMap() got %v, want empty", finalMap)
	}
}

func TestPatchSetForkCommit(t *testing.T) {
	// 1. Initialize empty patchset
	ps := NewPatchSet(NewPatch[string, int]())
	if initialMap := ps.AsMap(); len(initialMap) != 0 {
		t.Fatalf("Initial AsMap() got %v, want empty", initialMap)
	}

	// 2. Call Fork two times
	ps.Fork()
	ps.Fork()
	if len(ps.patches) != 3 {
		t.Fatalf("Expected 3 patches after 2xFork(), got %d", len(ps.patches))
	}

	// 3. Perform some mutation operations on the current layer
	ps.SetCurrent("a", 1)
	ps.SetCurrent("b", 2)
	ps.DeleteCurrent("a")

	// 4. Call Commit to persist changes
	ps.Commit()
	if len(ps.patches) != 2 {
		t.Fatalf("Expected 1 patch after Commit(), got %d", len(ps.patches))
	}

	// 5. Call Revert on the empty layer
	ps.Revert()
	if len(ps.patches) != 1 {
		t.Fatalf("Expected 1 patch after Revert(), got %d", len(ps.patches))
	}

	// 6. Compare state to the empty map
	wantMap := map[string]int{"b": 2}
	if finalMap := ps.AsMap(); maps.Equal(wantMap, finalMap) {
		t.Errorf("AsMap() got %v, want %v", finalMap, wantMap)
	}
}

func TestPatchSetFindValue(t *testing.T) {
	tests := map[string]struct {
		patchLayers []map[string]*int
		searchKey   string
		wantValue   int
		wantFound   bool
	}{
		"EmptyPatchSet_KeyNotFound": {
			patchLayers: []map[string]*int{},
			searchKey:   "a",
			wantFound:   false,
		},
		"SingleLayer_KeyFound": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}},
			searchKey:   "a",
			wantValue:   1,
			wantFound:   true,
		},
		"SingleLayer_KeyNotFound": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}},
			searchKey:   "x",
			wantFound:   false,
		},
		"SingleLayer_KeyDeleted": {
			patchLayers: []map[string]*int{{"a": nil, "b": ptr.To(2)}},
			searchKey:   "a",
			wantFound:   false,
		},
		"MultiLayer_KeyInTopLayer": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"a": ptr.To(10), "b": ptr.To(2)}},
			searchKey:   "a",
			wantValue:   10,
			wantFound:   true,
		},
		"MultiLayer_KeyInBottomLayer": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}, {"b": ptr.To(22)}},
			searchKey:   "a",
			wantValue:   1,
			wantFound:   true,
		},
		"MultiLayer_KeyOverwrittenThenDeleted": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"a": ptr.To(10)}, {"a": nil, "b": ptr.To(2)}},
			searchKey:   "a",
			wantFound:   false,
		},
		"MultiLayer_KeyDeletedThenAddedBack": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"a": nil}, {"a": ptr.To(100), "b": ptr.To(2)}},
			searchKey:   "a",
			wantValue:   100,
			wantFound:   true,
		},
		"MultiLayer_KeyNotFoundInAnyLayer": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"b": ptr.To(2)}},
			searchKey:   "x",
			wantFound:   false,
		},
		"MultiLayer_KeyDeletedInMiddleLayer_NotFound": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"a": nil, "b": ptr.To(2)}, {"c": ptr.To(3)}},
			searchKey:   "a",
			wantFound:   false,
		},
		"MultiLayer_KeyPresentInBase_DeletedInOverlay_ReAddedInTop": {
			patchLayers: []map[string]*int{
				{"k1": ptr.To(1)},
				{"k1": nil},
				{"k1": ptr.To(111)},
			},
			searchKey: "k1",
			wantValue: 111,
			wantFound: true,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			ps := buildTestPatchSet(t, tc.patchLayers)
			val, found := ps.FindValue(tc.searchKey)

			if found != tc.wantFound || (found && val != tc.wantValue) {
				t.Errorf("FindValue(%q) got val=%v, found=%v; want val=%v, found=%v", tc.searchKey, val, found, tc.wantValue, tc.wantFound)
			}
		})
	}
}

func TestPatchSetOperations(t *testing.T) {
	tests := map[string]struct {
		patchLayers    []map[string]*int
		mutatePatchSet func(ps *PatchSet[string, int])
		searchKey      string
		wantInCurrent  bool
	}{
		"SetCurrent_NewKey_InitiallyEmptyPatchSet": {
			patchLayers: []map[string]*int{},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.SetCurrent("a", 1)
			},
			searchKey:     "a",
			wantInCurrent: true,
		},
		"SetCurrent_NewKey_OnExistingEmptyLayer": {
			patchLayers: []map[string]*int{{}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.SetCurrent("a", 1)
			},
			searchKey:     "a",
			wantInCurrent: true,
		},
		"SetCurrent_OverwriteKey_InCurrentPatch": {
			patchLayers: []map[string]*int{{"a": ptr.To(10)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.SetCurrent("a", 1)
			},
			searchKey:     "a",
			wantInCurrent: true,
		},
		"SetCurrent_OverwriteKey_InLowerPatch_AfterFork": {
			patchLayers: []map[string]*int{{"a": ptr.To(10)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.Fork()
				ps.SetCurrent("a", 1)
			},
			searchKey:     "a",
			wantInCurrent: true,
		},
		"DeleteCurrent_ExistingKey_InCurrentPatch": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.DeleteCurrent("a")
			},
			searchKey:     "a",
			wantInCurrent: false,
		},
		"DeleteCurrent_ExistingKey_InLowerPatch_AfterFork": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.Fork()
				ps.DeleteCurrent("a")
			},
			searchKey:     "a",
			wantInCurrent: false,
		},
		"DeleteCurrent_NonExistentKey_OnExistingEmptyLayer": {
			patchLayers: []map[string]*int{{}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.DeleteCurrent("x")
			},
			searchKey:     "x",
			wantInCurrent: false,
		},
		"DeleteCurrent_NonExistentKey_InitiallyEmptyPatchSet": {
			patchLayers: []map[string]*int{}, // Starts with no layers
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.DeleteCurrent("x")
			},
			searchKey:     "x",
			wantInCurrent: false,
		},
		"InCurrentPatch_KeySetInCurrent": {
			patchLayers:    []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {},
			searchKey:      "a",
			wantInCurrent:  true,
		},
		"InCurrentPatch_KeySetInLower_NotInCurrent_AfterFork": {
			patchLayers:    []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) { ps.Fork() },
			searchKey:      "a",
			wantInCurrent:  false,
		},
		"InCurrentPatch_KeyDeletedInCurrent": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.DeleteCurrent("a")
			},
			searchKey:     "a",
			wantInCurrent: false, // Get in current patch won't find it.
		},
		"InCurrentPatch_NonExistentKey_InitiallyEmptyPatchSet": {
			patchLayers:    []map[string]*int{},
			mutatePatchSet: func(ps *PatchSet[string, int]) {},
			searchKey:      "a",
			wantInCurrent:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ps := buildTestPatchSet(t, tc.patchLayers)
			tc.mutatePatchSet(ps)

			inCurrent := ps.InCurrentPatch(tc.searchKey)
			if inCurrent != tc.wantInCurrent {
				t.Errorf("InCurrentPatch(%q): got %v, want %v", tc.searchKey, inCurrent, tc.wantInCurrent)
			}
		})
	}
}

func TestPatchSetCache(t *testing.T) {
	tests := map[string]struct {
		patchLayers     []map[string]*int
		mutatePatchSet  func(ps *PatchSet[string, int])
		wantCache       map[string]*int
		wantCacheInSync bool
	}{
		"Initial_EmptyPatchSet": {
			patchLayers:     []map[string]*int{},
			mutatePatchSet:  func(ps *PatchSet[string, int]) {},
			wantCache:       map[string]*int{},
			wantCacheInSync: true,
		},
		"Initial_OnlyDeletions_CacheInSync": {
			patchLayers:     []map[string]*int{{"a": nil}, {"b": nil}},
			mutatePatchSet:  func(ps *PatchSet[string, int]) {},
			wantCache:       map[string]*int{},
			wantCacheInSync: true,
		},
		"Initial_WithData_NoCacheAccess": {
			patchLayers:     []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet:  func(ps *PatchSet[string, int]) {},
			wantCache:       map[string]*int{},
			wantCacheInSync: false,
		},
		"FindValue_PopulatesCacheForKey": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("a")
			},
			wantCache:       map[string]*int{"a": ptr.To(1)},
			wantCacheInSync: false,
		},
		"FindValue_DeletedKey_PopulatesCacheWithNil": {
			patchLayers: []map[string]*int{{"a": nil, "b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("a")
			},
			wantCache:       map[string]*int{"a": nil},
			wantCacheInSync: false,
		},
		"AsMap_PopulatesAndSyncsCache": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": nil, "c": ptr.To(3)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
			},
			wantCache:       map[string]*int{"a": ptr.To(1), "c": ptr.To(3)}, // Cache does not necessarily track deletions like 'b' key
			wantCacheInSync: true,
		},
		"SetCurrent_UpdatesCache_NewKey": {
			patchLayers: []map[string]*int{{}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.SetCurrent("x", 10)
			},
			wantCache:       map[string]*int{"x": ptr.To(10)},
			wantCacheInSync: true,
		},
		"SetCurrent_UpdatesCache_OverwriteKey": {
			patchLayers: []map[string]*int{{"x": ptr.To(5)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("x")
				ps.SetCurrent("x", 10)
			},
			wantCache:       map[string]*int{"x": ptr.To(10)},
			wantCacheInSync: false,
		},
		"DeleteCurrent_UpdatesCache": {
			patchLayers: []map[string]*int{{"x": ptr.To(5)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("x")
				ps.DeleteCurrent("x")
			},
			wantCache:       map[string]*int{"x": nil},
			wantCacheInSync: false,
		},
		"Revert_ClearsAffectedCacheEntries_And_SetsCacheNotInSync": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"b": ptr.To(2), "a": ptr.To(11)}}, // Layer 0: a=1; Layer 1: b=2, a=11
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("a")
				ps.FindValue("b")
				ps.Revert()
			},
			wantCache:       map[string]*int{},
			wantCacheInSync: false,
		},
		"Revert_OnSyncedCache_RepairsCacheAndKeepsInSync": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.Revert()
			},
			wantCache:       map[string]*int{"a": ptr.To(1)},
			wantCacheInSync: true,
		},
		"Revert_OnSyncedCache_RestoresOverwrittenAndDeletedKeys": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.Fork()
				ps.SetCurrent("a", 11)
				ps.DeleteCurrent("b")
				ps.SetCurrent("c", 3)
				ps.DeleteCurrent("never-existed")
				ps.Revert()
			},
			wantCache:       map[string]*int{"a": ptr.To(1), "b": ptr.To(2)},
			wantCacheInSync: true,
		},
		"Revert_OnSyncedCache_ResolvesFromMiddleLayer": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}, {"a": ptr.To(11), "b": nil}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.Fork()
				ps.DeleteCurrent("a")
				ps.SetCurrent("b", 22)
				ps.Revert()
			},
			wantCache:       map[string]*int{"a": ptr.To(11)},
			wantCacheInSync: true,
		},
		"FindValue_MissOnSyncedCache_DoesNotStoreNegativeEntry": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.FindValue("x")
			},
			wantCache:       map[string]*int{"a": ptr.To(1)},
			wantCacheInSync: true,
		},
		"DeleteCurrent_OnSyncedCache_RemovesEntry": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.Fork()
				ps.DeleteCurrent("a")
			},
			wantCache:       map[string]*int{"b": ptr.To(2)},
			wantCacheInSync: true,
		},
		"AsMap_DropsNegativeEntries": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": nil}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("b")
				ps.FindValue("x")
				ps.AsMap()
			},
			wantCache:       map[string]*int{"a": ptr.To(1)},
			wantCacheInSync: true,
		},
		"Len_SyncsCache": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}, {"b": nil}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.Len()
			},
			wantCache:       map[string]*int{"a": ptr.To(1)},
			wantCacheInSync: true,
		},
		"Commit_DoesNotInvalidateCache_IfValuesConsistent": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("a")
				ps.FindValue("b")
				ps.Commit()
			},
			wantCache:       map[string]*int{"a": ptr.To(1), "b": ptr.To(2)},
			wantCacheInSync: false,
		},
		"Commit_OnSyncedCache_KeepsCacheInSync": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}, {"b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.Commit()
			},
			wantCache:       map[string]*int{"a": ptr.To(1), "b": ptr.To(2)},
			wantCacheInSync: true,
		},
		"Fork_DoesNotInvalidateCache": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("a")
				ps.Fork()
			},
			wantCache:       map[string]*int{"a": ptr.To(1)},
			wantCacheInSync: false,
		},
		"Fork_OnSyncedCache_KeepsCacheInSync": {
			patchLayers: []map[string]*int{{"a": ptr.To(1)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.Fork()
			},
			wantCache:       map[string]*int{"a": ptr.To(1)},
			wantCacheInSync: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ps := buildTestPatchSet(t, tc.patchLayers)
			tc.mutatePatchSet(ps)

			if !maps.EqualFunc(ps.cache, tc.wantCache, func(a, b *int) bool {
				if a == nil && b == nil {
					return true
				}
				if a == nil || b == nil {
					return false
				}
				return *a == *b
			}) {
				t.Errorf("Cache content mismatch: got %v, want %v", ps.cache, tc.wantCache)
			}

			if ps.cacheInSync != tc.wantCacheInSync {
				t.Errorf("cacheInSync status mismatch: got %v, want %v", ps.cacheInSync, tc.wantCacheInSync)
			}
		})
	}
}

func buildTestPatchSet[K comparable, V any](t *testing.T, patchLayers []map[K]*V) *PatchSet[K, V] {
	t.Helper()

	patchesNumber := len(patchLayers)
	patches := make([]*Patch[K, V], patchesNumber)
	for i := 0; i < patchesNumber; i++ {
		layerMap := patchLayers[i]
		currentPatch := NewPatch[K, V]()
		for k, vPtr := range layerMap {
			if vPtr != nil {
				currentPatch.Set(k, *vPtr)
			} else {
				currentPatch.Delete(k)
			}
		}
		patches[i] = currentPatch
	}

	return NewPatchSet(patches...)
}

func TestPatchSetNestedForkRevertKeepsCacheInSync(t *testing.T) {
	ps := buildTestPatchSet(t, []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2), "c": ptr.To(3)}})

	// assertState verifies the effective state through all read APIs and
	// that the cache stays in sync once it got synced by the first read.
	assertState := func(step string, want map[string]int) {
		t.Helper()
		if got := ps.AsMap(); !maps.Equal(got, want) {
			t.Errorf("%s: AsMap() got %v, want %v", step, got, want)
		}
		if !ps.cacheInSync {
			t.Errorf("%s: cacheInSync got false, want true", step)
		}
		gotValues := ps.ValuesSlice()
		slices.Sort(gotValues)
		wantValues := slices.Sorted(maps.Values(want))
		if !slices.Equal(gotValues, wantValues) {
			t.Errorf("%s: ValuesSlice() got %v, want %v", step, gotValues, wantValues)
		}
		if iterValues := slices.Sorted(ps.Values()); !slices.Equal(iterValues, wantValues) {
			t.Errorf("%s: Values() got %v, want %v", step, iterValues, wantValues)
		}
		if got := ps.Len(); got != len(want) {
			t.Errorf("%s: Len() got %d, want %d", step, got, len(want))
		}
		for _, key := range []string{"a", "b", "c", "d", "e", "missing"} {
			wantValue, wantFound := want[key]
			gotValue, gotFound := ps.FindValue(key)
			if gotFound != wantFound || gotValue != wantValue {
				t.Errorf("%s: FindValue(%q) got (%v, %v), want (%v, %v)", step, key, gotValue, gotFound, wantValue, wantFound)
			}
		}
		for key, value := range ps.cache {
			if value == nil {
				t.Errorf("%s: in-sync cache contains a negative entry for key %q", step, key)
			}
		}
	}

	assertState("initial", map[string]int{"a": 1, "b": 2, "c": 3})

	ps.Fork()
	ps.SetCurrent("a", 10)
	ps.SetCurrent("d", 4)
	assertState("fork#1", map[string]int{"a": 10, "b": 2, "c": 3, "d": 4})

	ps.Fork()
	ps.SetCurrent("b", 20)
	ps.DeleteCurrent("a")
	ps.DeleteCurrent("c")
	ps.SetCurrent("e", 5)
	assertState("fork#2", map[string]int{"b": 20, "d": 4, "e": 5})

	ps.Revert()
	assertState("revert fork#2", map[string]int{"a": 10, "b": 2, "c": 3, "d": 4})

	ps.Fork()
	ps.DeleteCurrent("d")
	ps.SetCurrent("c", 30)
	assertState("fork#2'", map[string]int{"a": 10, "b": 2, "c": 30})

	ps.Commit()
	assertState("commit fork#2'", map[string]int{"a": 10, "b": 2, "c": 30})

	ps.Revert()
	assertState("revert fork#1", map[string]int{"a": 1, "b": 2, "c": 3})

	// Reverting the base layer is a no-op.
	ps.Revert()
	assertState("revert base", map[string]int{"a": 1, "b": 2, "c": 3})

	// Deep fork stack with the same key touched at every level:
	// layers: a=100, del(a), a=102, del(a), a=104.
	for i := 0; i < 5; i++ {
		ps.Fork()
		if i%2 == 0 {
			ps.SetCurrent("a", 100+i)
		} else {
			ps.DeleteCurrent("a")
		}
	}
	assertState("deep fork", map[string]int{"a": 104, "b": 2, "c": 3})
	ps.Revert()
	assertState("deep revert#1", map[string]int{"b": 2, "c": 3})
	ps.Revert()
	assertState("deep revert#2", map[string]int{"a": 102, "b": 2, "c": 3})
	ps.Revert()
	assertState("deep revert#3", map[string]int{"b": 2, "c": 3})
	ps.Revert()
	assertState("deep revert#4", map[string]int{"a": 100, "b": 2, "c": 3})
	ps.Revert()
	assertState("deep revert#5", map[string]int{"a": 1, "b": 2, "c": 3})
}

func TestPatchSetValuesSliceAndLen(t *testing.T) {
	tests := map[string]struct {
		patchLayers    []map[string]*int
		mutatePatchSet func(ps *PatchSet[string, int])
		wantValues     []int
	}{
		"Empty_NoLayers": {
			patchLayers:    []map[string]*int{},
			mutatePatchSet: func(ps *PatchSet[string, int]) {},
			wantValues:     []int{},
		},
		"Empty_SingleEmptyLayer": {
			patchLayers:    []map[string]*int{{}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {},
			wantValues:     []int{},
		},
		"Populated_MultipleLayers": {
			patchLayers:    []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}, {"b": nil, "c": ptr.To(3)}, {"a": ptr.To(11)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {},
			wantValues:     []int{3, 11},
		},
		"Populated_AfterSetAndDeleteOnSyncedCache": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.Fork()
				ps.SetCurrent("c", 3)
				ps.DeleteCurrent("a")
			},
			wantValues: []int{2, 3},
		},
		"Reverted_OnSyncedCache": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.AsMap()
				ps.Fork()
				ps.SetCurrent("c", 3)
				ps.DeleteCurrent("a")
				ps.Revert()
			},
			wantValues: []int{1, 2},
		},
		"Reverted_OnNotSyncedCache": {
			patchLayers: []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2)}, {"a": nil, "c": ptr.To(3)}},
			mutatePatchSet: func(ps *PatchSet[string, int]) {
				ps.FindValue("a")
				ps.FindValue("c")
				ps.Revert()
			},
			wantValues: []int{1, 2},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ps := buildTestPatchSet(t, tc.patchLayers)
			tc.mutatePatchSet(ps)

			if got := ps.Len(); got != len(tc.wantValues) {
				t.Errorf("Len() got %d, want %d", got, len(tc.wantValues))
			}
			gotValues := ps.ValuesSlice()
			if gotValues == nil {
				t.Errorf("ValuesSlice() got nil, want non-nil slice")
			}
			slices.Sort(gotValues)
			if !slices.Equal(gotValues, tc.wantValues) {
				t.Errorf("ValuesSlice() got %v, want %v", gotValues, tc.wantValues)
			}
			if iterValues := slices.Sorted(ps.Values()); !slices.Equal(iterValues, tc.wantValues) {
				t.Errorf("Values() got %v, want %v", iterValues, tc.wantValues)
			}
		})
	}
}

func TestPatchSetValuesEarlyBreak(t *testing.T) {
	ps := buildTestPatchSet(t, []map[string]*int{{"a": ptr.To(1), "b": ptr.To(2), "c": ptr.To(3)}})
	count := 0
	for range ps.Values() {
		count++
		if count == 2 {
			break
		}
	}
	if count != 2 {
		t.Errorf("Values() early break: got %d iterations, want 2", count)
	}
}

func TestClonePatchSetCacheState(t *testing.T) {
	ps := buildTestPatchSet(t, []map[string]*int{{"a": ptr.To(1)}})
	ps.AsMap()
	ps.Fork()
	ps.SetCurrent("b", 2)

	cloned := ClonePatchSet(ps, func(k string) string { return k }, func(v int) int { return v })
	want := map[string]int{"a": 1, "b": 2}
	if got := cloned.AsMap(); !maps.Equal(got, want) {
		t.Errorf("cloned AsMap() got %v, want %v", got, want)
	}
	cloned.Revert()
	if got, want := cloned.AsMap(), map[string]int{"a": 1}; !maps.Equal(got, want) {
		t.Errorf("cloned AsMap() after Revert() got %v, want %v", got, want)
	}
	if got := ps.AsMap(); !maps.Equal(got, want) {
		t.Errorf("original AsMap() changed after reverting the clone: got %v, want %v", got, want)
	}
}

// referencePatchSet is a deliberately naive model of PatchSet used for
// differential testing: it stores the full effective state for every layer.
type referencePatchSet struct {
	states []map[int]int
}

func (r *referencePatchSet) top() map[int]int {
	if len(r.states) == 0 {
		r.states = append(r.states, map[int]int{})
	}
	return r.states[len(r.states)-1]
}

func (r *referencePatchSet) effective() map[int]int {
	if len(r.states) == 0 {
		return map[int]int{}
	}
	return r.states[len(r.states)-1]
}

func (r *referencePatchSet) fork() {
	r.states = append(r.states, maps.Clone(r.effective()))
}

func (r *referencePatchSet) commit() {
	if len(r.states) < 2 {
		return
	}
	r.states[len(r.states)-2] = r.states[len(r.states)-1]
	r.states = r.states[:len(r.states)-1]
}

func (r *referencePatchSet) revert() {
	if len(r.states) <= 1 {
		return
	}
	r.states = r.states[:len(r.states)-1]
}

// patchSetOpReader turns a byte stream into a sequence of operation arguments.
type patchSetOpReader struct {
	data []byte
}

func (r *patchSetOpReader) next() (byte, bool) {
	if len(r.data) == 0 {
		return 0, false
	}
	b := r.data[0]
	r.data = r.data[1:]
	return b, true
}

const differentialKeySpace = 16

// runPatchSetDifferential interprets data as a sequence of PatchSet operations and
// compares the results of all read operations against referencePatchSet after each step.
func runPatchSetDifferential(t *testing.T, data []byte) {
	t.Helper()
	r := &patchSetOpReader{data: data}
	ref := &referencePatchSet{}

	// Initial layers passed directly to NewPatchSet.
	layersCount, _ := r.next()
	var patches []*Patch[int, int]
	for range layersCount % 4 {
		patch := NewPatch[int, int]()
		entries, _ := r.next()
		for range entries % 8 {
			key, _ := r.next()
			value, _ := r.next()
			if value%4 == 0 {
				patch.Delete(int(key % differentialKeySpace))
			} else {
				patch.Set(int(key%differentialKeySpace), int(value))
			}
		}
		patches = append(patches, patch)
		state := maps.Clone(ref.effective())
		for key, value := range patch.modified {
			state[key] = value
		}
		for key := range patch.deleted {
			delete(state, key)
		}
		ref.states = append(ref.states, state)
	}
	ps := NewPatchSet(patches...)

	var history []string
	fail := func(format string, args ...any) {
		t.Helper()
		const maxHistory = 30
		recent := history
		if len(recent) > maxHistory {
			recent = recent[len(recent)-maxHistory:]
		}
		t.Fatalf("after %d ops (last %d: %v): "+format, append([]any{len(history), len(recent), recent}, args...)...)
	}

	checkCacheInvariants := func() {
		t.Helper()
		want := ref.effective()
		for key, cached := range ps.cache {
			wantValue, wantFound := want[key]
			if cached == nil {
				if ps.cacheInSync {
					fail("in-sync cache holds a negative entry for key %d", key)
				}
				if wantFound {
					fail("cache holds a negative entry for existing key %d (want %d)", key, wantValue)
				}
				continue
			}
			if !wantFound || *cached != wantValue {
				fail("stale cache entry for key %d: got %d, want (%d, %v)", key, *cached, wantValue, wantFound)
			}
		}
		if ps.cacheInSync && len(ps.cache) != len(want) {
			fail("in-sync cache has %d entries, want %d", len(ps.cache), len(want))
		}
	}

	for {
		opByte, ok := r.next()
		if !ok {
			break
		}
		argKey, _ := r.next()
		key := int(argKey % differentialKeySpace)

		switch opByte % 12 {
		case 0:
			history = append(history, "Fork")
			ps.Fork()
			ref.fork()
		case 1:
			history = append(history, "Commit")
			ps.Commit()
			ref.commit()
		case 2, 3:
			history = append(history, "Revert")
			ps.Revert()
			ref.revert()
		case 4, 5, 6:
			value, _ := r.next()
			history = append(history, fmt.Sprintf("Set(%d,%d)", key, value))
			ps.SetCurrent(key, int(value))
			ref.top()[key] = int(value)
		case 7:
			history = append(history, fmt.Sprintf("Delete(%d)", key))
			ps.DeleteCurrent(key)
			delete(ref.top(), key)
		case 8:
			history = append(history, fmt.Sprintf("Find(%d)", key))
			gotValue, gotFound := ps.FindValue(key)
			wantValue, wantFound := ref.effective()[key]
			if gotValue != wantValue || gotFound != wantFound {
				fail("FindValue(%d) got (%d, %v), want (%d, %v)", key, gotValue, gotFound, wantValue, wantFound)
			}
		case 9:
			history = append(history, "AsMap")
			if got, want := ps.AsMap(), ref.effective(); !maps.Equal(got, want) {
				fail("AsMap() got %v, want %v", got, want)
			}
		case 10:
			history = append(history, "ValuesSlice")
			got := ps.ValuesSlice()
			slices.Sort(got)
			if want := slices.Sorted(maps.Values(ref.effective())); !slices.Equal(got, want) {
				fail("ValuesSlice() got %v, want %v", got, want)
			}
			if got := slices.Sorted(ps.Values()); !slices.Equal(got, slices.Sorted(maps.Values(ref.effective()))) {
				fail("Values() got %v", got)
			}
		case 11:
			history = append(history, "Len")
			if got, want := ps.Len(), len(ref.effective()); got != want {
				fail("Len() got %d, want %d", got, want)
			}
		}

		if len(ps.patches) != len(ref.states) {
			fail("got %d patches, want %d", len(ps.patches), len(ref.states))
		}
		checkCacheInvariants()
	}

	// Final full comparison.
	if got, want := ps.AsMap(), ref.effective(); !maps.Equal(got, want) {
		fail("final AsMap() got %v, want %v", got, want)
	}
}

// TestPatchSetDifferential runs a large number of pseudo-random operations against
// PatchSet and a naive reference implementation, comparing them at every step.
func TestPatchSetDifferential(t *testing.T) {
	const seeds = 20
	const opsPerSeed = 5000
	for seed := uint64(0); seed < seeds; seed++ {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, 0x5eed))
			// ~3 bytes per op on average.
			data := make([]byte, opsPerSeed*3)
			for i := range data {
				data[i] = byte(rng.UintN(256))
			}
			runPatchSetDifferential(t, data)
		})
	}
}

func FuzzPatchSetDifferential(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0})
	f.Add([]byte{2, 3, 1, 1, 5, 2, 0, 4, 0, 2, 9, 0, 0, 1, 7, 1, 2, 0, 9, 0})
	f.Add([]byte{1, 2, 3, 5, 7, 9, 0, 0, 4, 3, 33, 7, 5, 8, 5, 2, 0, 8, 5, 10, 0, 11, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		runPatchSetDifferential(t, data)
	})
}

var benchSizes = []int{1000, 10000}

func newBenchPatchSet(n int) *PatchSet[int, *int] {
	base := make(map[int]*int, n)
	for i := 0; i < n; i++ {
		v := i
		base[i] = &v
	}
	ps := NewPatchSet(NewPatchFromMap(base))
	ps.AsMap() // sync the cache, as happens after the first List in a loop
	return ps
}

func listViaAsMap(ps *PatchSet[int, *int]) []*int {
	m := ps.AsMap()
	res := make([]*int, 0, len(m))
	for _, v := range m {
		res = append(res, v)
	}
	return res
}

var benchSink []*int
var benchFoundSink bool

// BenchmarkPatchSetRevert measures a typical simulation cycle:
// Fork -> modify a handful of keys -> Revert -> (optionally) list all values.
func BenchmarkPatchSetRevert(b *testing.B) {
	modify := func(ps *PatchSet[int, *int], i, n int) {
		ps.Fork()
		for k := 0; k < 5; k++ {
			v := k
			ps.SetCurrent((i+k)%n, &v)
		}
		ps.DeleteCurrent((i + 7) % n)
	}
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("N=%d/ForkModifyRevert", n), func(b *testing.B) {
			ps := newBenchPatchSet(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				modify(ps, i, n)
				ps.Revert()
			}
		})
		b.Run(fmt.Sprintf("N=%d/ForkModifyRevertAsMapList", n), func(b *testing.B) {
			ps := newBenchPatchSet(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				modify(ps, i, n)
				ps.Revert()
				benchSink = listViaAsMap(ps)
			}
		})
		b.Run(fmt.Sprintf("N=%d/ForkModifyRevertValuesSlice", n), func(b *testing.B) {
			ps := newBenchPatchSet(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				modify(ps, i, n)
				ps.Revert()
				benchSink = ps.ValuesSlice()
			}
		})
	}
}

// BenchmarkPatchSetValuesSlice compares listing values through AsMap() vs ValuesSlice() / Values().
func BenchmarkPatchSetValuesSlice(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("N=%d/AsMapToSlice", n), func(b *testing.B) {
			ps := newBenchPatchSet(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = listViaAsMap(ps)
			}
		})
		b.Run(fmt.Sprintf("N=%d/ValuesSlice", n), func(b *testing.B) {
			ps := newBenchPatchSet(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSink = ps.ValuesSlice()
			}
		})
		b.Run(fmt.Sprintf("N=%d/ValuesIter", n), func(b *testing.B) {
			ps := newBenchPatchSet(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				count := 0
				for range ps.Values() {
					count++
				}
				benchFoundSink = count == n
			}
		})
	}
}

// BenchmarkPatchSetColdAsMap measures the first full read of a freshly created PatchSet,
// which needs to build the cache from scratch.
func BenchmarkPatchSetColdAsMap(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			base := make(map[int]*int, n)
			for i := 0; i < n; i++ {
				v := i
				base[i] = &v
			}
			patch := NewPatchFromMap(base)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ps := NewPatchSet(patch)
				benchSink = listViaAsMap(ps)
			}
		})
	}
}

// BenchmarkPatchSetFindValueMiss measures lookups of keys absent from the PatchSet.
func BenchmarkPatchSetFindValueMiss(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("N=%d/Forked", n), func(b *testing.B) {
			ps := newBenchPatchSet(n)
			ps.Fork()
			ps.Fork()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, benchFoundSink = ps.FindValue(n + i)
			}
		})
	}
}
