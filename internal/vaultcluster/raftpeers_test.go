package vaultcluster

import (
	"encoding/json"
	"testing"
)

const raftConfigurationJSON = `{"config":{"index":42,"servers":[
  {"address":"10.0.1.10:8201","leader":true,"node_id":"i-old","protocol_version":"3","voter":true},
  {"address":"10.0.2.20:8201","leader":false,"node_id":"i-new","protocol_version":"3","voter":true},
  {"address":"10.0.3.30:8201","leader":false,"node_id":"i-learner","protocol_version":"3","voter":false}]}}`

func raftConfiguration(t *testing.T) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(raftConfigurationJSON), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseRaftConfiguration(t *testing.T) {
	peers := parseRaftConfiguration(raftConfiguration(t))
	if len(peers) != 3 {
		t.Fatalf("got %d peers, want 3", len(peers))
	}
	if !peers[0].Leader || !peers[0].Voter || peers[0].ID != "i-old" || peers[0].Address != "10.0.1.10:8201" {
		t.Fatalf("leader parsed wrong: %+v", peers[0])
	}
	if peers[2].Voter {
		t.Fatalf("learner must not be a voter: %+v", peers[2])
	}
}

func TestOtherVotersIgnoresSelfAndLearners(t *testing.T) {
	peers := parseRaftConfiguration(raftConfiguration(t))

	self, others := otherVoters(peers, "i-old")
	if self == nil || !self.Leader || others != 1 {
		t.Fatalf("old leader: self=%+v others=%d, want leader with 1 other voter", self, others)
	}

	self, others = otherVoters(peers, "i-new")
	if self == nil || self.Leader || others != 1 {
		t.Fatalf("new node: self=%+v others=%d, want follower with 1 other voter", self, others)
	}

	if self, _ := otherVoters(peers, "i-gone"); self != nil {
		t.Fatalf("a node outside the configuration must have no self entry")
	}

	single := []RaftPeer{{ID: "i-only", Leader: true, Voter: true}, {ID: "i-learner"}}
	if _, others := otherVoters(single, "i-only"); others != 0 {
		t.Fatalf("a lone voter with a learner has %d other voters, want 0", others)
	}
}

func TestParseRaftConfigurationEmpty(t *testing.T) {
	if peers := parseRaftConfiguration(map[string]any{}); len(peers) != 0 {
		t.Fatalf("got %v, want none", peers)
	}
}
