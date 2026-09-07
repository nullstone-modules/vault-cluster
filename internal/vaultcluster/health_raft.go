package vaultcluster

import (
	"encoding/json"
	"fmt"
	"strings"
)

type raftServer struct {
	ID      string
	Status  string
	Healthy bool
	Voter   bool
}

func parseAutopilot(data map[string]any) []raftServer {
	raw, _ := json.Marshal(data)
	var parsed struct {
		Voters  []string `json:"voters"`
		Servers map[string]struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Healthy bool   `json:"healthy"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	voters := map[string]bool{}
	for _, id := range parsed.Voters {
		voters[id] = true
	}
	var out []raftServer
	for id, s := range parsed.Servers {
		if s.ID == "" {
			s.ID = id
		}
		status := strings.ToLower(s.Status)
		out = append(out, raftServer{
			ID:      s.ID,
			Status:  status,
			Healthy: s.Healthy,
			Voter:   voters[s.ID] || status == "leader" || status == "voter",
		})
	}
	return out
}

func NodeRaftReady(nodeID string, data map[string]any) error {
	if nodeID == "" {
		return fmt.Errorf("VAULT_RAFT_NODE_ID is not set")
	}
	if data == nil {
		return fmt.Errorf("raft autopilot state is missing")
	}
	for _, s := range parseAutopilot(data) {
		if s.ID != nodeID {
			continue
		}
		if !s.Voter {
			return fmt.Errorf("node %s is not a raft voter", nodeID)
		}
		if !s.Healthy {
			return fmt.Errorf("node %s is not caught up", nodeID)
		}
		return nil
	}
	return fmt.Errorf("node %s is not in the raft cluster", nodeID)
}

func (c *Client) RaftAutopilot() (map[string]any, error) {
	sec, err := c.API.Logical().Read("sys/storage/raft/autopilot/state")
	if err != nil {
		return nil, err
	}
	if sec == nil || sec.Data == nil {
		return nil, fmt.Errorf("raft autopilot state is empty")
	}
	return sec.Data, nil
}

func (c *Client) NodeHealthOK(nodeID string) error {
	st, err := c.API.Sys().SealStatus()
	if err != nil {
		return err
	}
	if !st.Initialized || st.Sealed {
		return fmt.Errorf("vault is not ready")
	}
	data, err := c.RaftAutopilot()
	if err != nil {
		return err
	}
	return NodeRaftReady(nodeID, data)
}
