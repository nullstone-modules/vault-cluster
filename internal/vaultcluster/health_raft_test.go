package vaultcluster

import "testing"

func autopilot(nodeID, status string, healthy bool, voters []string) map[string]any {
	return map[string]any{
		"voters": voters,
		"servers": map[string]any{
			nodeID: map[string]any{
				"id":      nodeID,
				"status":  status,
				"healthy": healthy,
			},
		},
	}
}

func TestNodeRaftReadyVoterCaughtUp(t *testing.T) {
	data := autopilot("n1", "voter", true, []string{"n1", "n2"})
	if err := NodeRaftReady("n1", data); err != nil {
		t.Fatal(err)
	}
}

func TestNodeRaftReadyLeader(t *testing.T) {
	data := autopilot("n1", "leader", true, []string{"n1"})
	if err := NodeRaftReady("n1", data); err != nil {
		t.Fatal(err)
	}
}

func TestNodeRaftReadyNonVoter(t *testing.T) {
	data := autopilot("n3", "non-voter", true, []string{"n1", "n2"})
	if err := NodeRaftReady("n3", data); err == nil {
		t.Fatal("expected non-voter to fail")
	}
}

func TestNodeRaftReadyNotCaughtUp(t *testing.T) {
	data := autopilot("n1", "voter", false, []string{"n1"})
	if err := NodeRaftReady("n1", data); err == nil {
		t.Fatal("expected unhealthy voter to fail")
	}
}

func TestNodeRaftReadyMissingNode(t *testing.T) {
	data := autopilot("n1", "leader", true, []string{"n1"})
	if err := NodeRaftReady("n2", data); err == nil {
		t.Fatal("expected missing node to fail")
	}
}

func TestNodeRaftReadyRequiresNodeID(t *testing.T) {
	if err := NodeRaftReady("", autopilot("n1", "leader", true, []string{"n1"})); err == nil {
		t.Fatal("expected empty node id to fail")
	}
}
