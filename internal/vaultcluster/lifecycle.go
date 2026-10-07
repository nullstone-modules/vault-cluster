package vaultcluster

import (
	"context"
	"errors"
	"time"
)

// Auto Scaling lifecycle hooks make a launch-template change survivable on any cluster size, including one.
//
// The launch hook holds a new instance in Pending:Wait until this node is a caught-up Raft voter, so a
// rolling refresh never retires the old node before the new one holds the data. A node that never joins is
// abandoned, which fails the refresh and rolls it back with the old node untouched.
//
// The terminate hook holds a departing instance in Terminating:Wait until this node has left the peer set,
// so the survivors keep quorum. The hook continues whether or not the leave succeeded: ABANDON terminates
// too, and the attempt is logged.
const (
	LaunchHook           = "vault-join"
	TerminateHook        = "vault-leave"
	StatePendingWait     = "Pending:Wait"
	StateTerminatingWait = "Terminating:Wait"
)

var ErrNotInGroup = errors.New("instance is not in an auto scaling group")

type LifecycleAPI interface {
	State(ctx context.Context) (string, error)
	Complete(ctx context.Context, hook string) error
}

type LifecycleNode interface {
	// Ready is nil once this node is an unsealed, caught-up Raft voter.
	Ready() error
	// Leave removes this node from the Raft peer set. ErrSoleVoter means there was nothing to hand off.
	Leave() error
}

type LifecycleOptions struct {
	Poll         time.Duration
	LeaveTimeout time.Duration
	LeaveRetry   time.Duration
	Logf         func(string, ...any)
}

func (o *LifecycleOptions) defaults() {
	if o.Poll <= 0 {
		o.Poll = 10 * time.Second
	}
	if o.LeaveTimeout <= 0 {
		o.LeaveTimeout = 2 * time.Minute
	}
	if o.LeaveRetry <= 0 {
		o.LeaveRetry = 3 * time.Second
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
}

// RunLifecycle polls the instance's lifecycle state until ctx ends or the instance is not in a group.
func RunLifecycle(ctx context.Context, api LifecycleAPI, node LifecycleNode, opts LifecycleOptions) error {
	opts.defaults()
	left, continued := false, false
	for {
		state, err := api.State(ctx)
		switch {
		case errors.Is(err, ErrNotInGroup):
			return err
		case err != nil:
			opts.Logf("lifecycle state: %v", err)
		case state == StatePendingWait:
			if err := node.Ready(); err != nil {
				opts.Logf("waiting to join: %v", err)
			} else if err := api.Complete(ctx, LaunchHook); err != nil {
				opts.Logf("launch hook: %v", err)
			} else {
				opts.Logf("joined the raft cluster; launch hook continued")
			}
		case state == StateTerminatingWait && !continued:
			if !left {
				left = true
				leaveWithRetry(ctx, node, opts)
			}
			if err := api.Complete(ctx, TerminateHook); err != nil {
				opts.Logf("terminate hook: %v", err)
			} else {
				continued = true
				opts.Logf("terminate hook continued")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opts.Poll):
		}
	}
}

func leaveWithRetry(ctx context.Context, node LifecycleNode, opts LifecycleOptions) {
	ctx, cancel := context.WithTimeout(ctx, opts.LeaveTimeout)
	defer cancel()
	for {
		err := node.Leave()
		switch {
		case err == nil:
			opts.Logf("left the raft cluster")
			return
		case errors.Is(err, ErrSoleVoter):
			opts.Logf("%v", err)
			return
		}
		opts.Logf("leaving the raft cluster: %v", err)
		select {
		case <-ctx.Done():
			opts.Logf("giving up on leaving the raft cluster; terminating anyway")
			return
		case <-time.After(opts.LeaveRetry):
		}
	}
}
