package recall

import (
	"testing"
)

func TestSingleList(t *testing.T) {
	list := []MergedResult{
		{UID: "a1", Title: "A1", Content: "c1"},
		{UID: "a2", Title: "A2", Content: "c2"},
		{UID: "a3", Title: "A3", Content: "c3"},
	}

	result := InterleaveResults([][]MergedResult{list}, 10)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	for i, r := range result {
		if r.UID != list[i].UID {
			t.Errorf("result[%d]: expected UID %q, got %q", i, list[i].UID, r.UID)
		}
	}
}

func TestRoundRobin(t *testing.T) {
	listA := []MergedResult{
		{UID: "a1", Title: "A1"},
		{UID: "a2", Title: "A2"},
	}
	listB := []MergedResult{
		{UID: "b1", Title: "B1"},
		{UID: "b2", Title: "B2"},
	}

	result := InterleaveResults([][]MergedResult{listA, listB}, 10)
	if len(result) != 4 {
		t.Fatalf("expected 4 results, got %d", len(result))
	}

	expected := []string{"a1", "b1", "a2", "b2"}
	for i, uid := range expected {
		if result[i].UID != uid {
			t.Errorf("result[%d]: expected UID %q, got %q", i, uid, result[i].UID)
		}
	}
}

func TestUnevenLists(t *testing.T) {
	listA := []MergedResult{
		{UID: "a1", Title: "A1"},
		{UID: "a2", Title: "A2"},
		{UID: "a3", Title: "A3"},
	}
	listB := []MergedResult{
		{UID: "b1", Title: "B1"},
	}

	result := InterleaveResults([][]MergedResult{listA, listB}, 10)
	if len(result) != 4 {
		t.Fatalf("expected 4 results, got %d", len(result))
	}

	expected := []string{"a1", "b1", "a2", "a3"}
	for i, uid := range expected {
		if result[i].UID != uid {
			t.Errorf("result[%d]: expected UID %q, got %q", i, uid, result[i].UID)
		}
	}
}

func TestMaxResults(t *testing.T) {
	listA := make([]MergedResult, 5)
	listB := make([]MergedResult, 5)
	for i := 0; i < 5; i++ {
		listA[i] = MergedResult{UID: "a" + string(rune('1'+i))}
		listB[i] = MergedResult{UID: "b" + string(rune('1'+i))}
	}

	result := InterleaveResults([][]MergedResult{listA, listB}, 3)
	if len(result) != 3 {
		t.Fatalf("expected exactly 3 results, got %d", len(result))
	}
}

func TestDeduplication(t *testing.T) {
	listA := []MergedResult{
		{UID: "shared", Title: "From A"},
		{UID: "a-only", Title: "A Only"},
	}
	listB := []MergedResult{
		{UID: "shared", Title: "From B"},
		{UID: "b-only", Title: "B Only"},
	}

	result := InterleaveResults([][]MergedResult{listA, listB}, 10)
	// "shared" should appear only once (from list A, since it comes first in round-robin)
	uidCount := map[string]int{}
	for _, r := range result {
		uidCount[r.UID]++
	}
	if uidCount["shared"] != 1 {
		t.Errorf("expected 'shared' to appear once, appeared %d times", uidCount["shared"])
	}
	if len(result) != 3 {
		t.Errorf("expected 3 unique results, got %d", len(result))
	}
}

func TestEmptyLists(t *testing.T) {
	result := InterleaveResults(nil, 10)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}

	result = InterleaveResults([][]MergedResult{}, 10)
	if result != nil {
		t.Errorf("expected nil for zero lists, got %v", result)
	}
}

func TestSourceKBPreserved(t *testing.T) {
	listA := []MergedResult{
		{UID: "a1", Title: "A1", SourceKB: "kb-alpha"},
	}
	listB := []MergedResult{
		{UID: "b1", Title: "B1", SourceKB: "kb-beta"},
	}

	result := InterleaveResults([][]MergedResult{listA, listB}, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].SourceKB != "kb-alpha" {
		t.Errorf("result[0] SourceKB: expected %q, got %q", "kb-alpha", result[0].SourceKB)
	}
	if result[1].SourceKB != "kb-beta" {
		t.Errorf("result[1] SourceKB: expected %q, got %q", "kb-beta", result[1].SourceKB)
	}
}
