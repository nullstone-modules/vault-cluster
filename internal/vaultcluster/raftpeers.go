package vaultcluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// A leader hands off before leaving, and the new leader needs a moment to win the election.
const leaderHandoffTimeout = 20 * time.Second

// ErrSoleVoter: this node is the only voter, so there is nobody to hand the data to.
// Removing it would only make the cluster unreachable sooner, so it is left in place.
var ErrSoleVoter = errors.New("this node is the only raft voter; leaving the peer set unchanged")

type RaftPeer struct {
	ID      string
	Address string
	Leader  bool
	Voter   bool
}

func parseRaftConfiguration(data map[string]any) []RaftPeer {
	raw, _ := json.Marshal(data)
	var parsed struct {
		Config struct {
			Servers []struct {
				ID      string `json:"node_id"`
				Address string `json:"address"`
				Leader  bool   `json:"leader"`
				Voter   bool   `json:"voter"`
			} `json:"servers"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	out := make([]RaftPeer, 0, len(parsed.Config.Servers))
	for _, s := range parsed.Config.Servers {
		out = append(out, RaftPeer{ID: s.ID, Address: s.Address, Leader: s.Leader, Voter: s.Voter})
	}
	return out
}

// otherVoters counts voters besides nodeID and returns nodeID's own entry, or nil when it is not a peer.
func otherVoters(peers []RaftPeer, nodeID string) (*RaftPeer, int) {
	var self *RaftPeer
	n := 0
	for i := range peers {
		if peers[i].ID == nodeID {
			self = &peers[i]
			continue
		}
		if peers[i].Voter {
			n++
		}
	}
	return self, n
}

func (c *Client) RaftPeers() ([]RaftPeer, error) {
	sec, err := c.API.Logical().Read("sys/storage/raft/configuration")
	if err != nil {
		return nil, err
	}
	if sec == nil || sec.Data == nil {
		return nil, fmt.Errorf("raft configuration is empty")
	}
	return parseRaftConfiguration(sec.Data), nil
}

// LeaveRaft removes this node from the peer set so the remaining voters keep quorum.
// A leader steps down first, so the removal is committed by a node that stays.
func (c *Client) LeaveRaft(nodeID string) error {
	peers, err := c.RaftPeers()
	if err != nil {
		return err
	}
	self, others := otherVoters(peers, nodeID)
	if self == nil {
		return nil
	}
	if others == 0 {
		return ErrSoleVoter
	}
	if self.Leader {
		if err := c.API.Sys().StepDown(); err != nil {
			return fmt.Errorf("step down: %w", err)
		}
		if err := c.waitOtherLeader(leaderHandoffTimeout); err != nil {
			return err
		}
	}
	if _, err := c.API.Logical().Write("sys/storage/raft/remove-peer", map[string]any{"server_id": nodeID}); err != nil {
		return fmt.Errorf("remove peer %s: %w", nodeID, err)
	}
	// A removed node may refuse further requests; that is also confirmation.
	peers, err = c.RaftPeers()
	if err != nil {
		return nil
	}
	if self, _ := otherVoters(peers, nodeID); self != nil {
		return fmt.Errorf("node %s is still a raft peer after remove-peer", nodeID)
	}
	return nil
}

func (c *Client) waitOtherLeader(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		l, err := c.API.Sys().Leader()
		if err == nil && !l.IsSelf && l.LeaderAddress != "" {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("no other node took leadership within %s", timeout)
}
