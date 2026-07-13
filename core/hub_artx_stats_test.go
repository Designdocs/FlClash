package main

import (
	"encoding/json"
	"testing"
)

func TestTrafficSnapshotIncludesArtXDisconnectCounter(t *testing.T) {
	var snapshot map[string]int64
	if err := json.Unmarshal([]byte(handleGetTraffic(false)), &snapshot); err != nil {
		t.Fatal(err)
	}
	if _, ok := snapshot["artx_unexpected_disconnects"]; !ok {
		t.Fatalf("traffic snapshot = %#v", snapshot)
	}
}
