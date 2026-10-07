package vaultcluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeLifecycleAPI struct {
	mu        sync.Mutex
	states    []string
	completed []string
	failHook  string
}

func (f *fakeLifecycleAPI) State(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) == 0 {
		return "", ErrNotInGroup
	}
	s := f.states[0]
	if len(f.states) > 1 {
		f.states = f.states[1:]
	}
	return s, nil
}

func (f *fakeLifecycleAPI) Complete(_ context.Context, hook string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if hook == f.failHook {
		f.failHook = ""
		return errors.New("throttled")
	}
	f.completed = append(f.completed, hook)
	if hook == LaunchHook {
		f.states = []string{"InService"}
	}
	if hook == TerminateHook {
		f.states = []string{"Terminating:Proceed"}
	}
	return nil
}

type fakeNode struct {
	readyAfter int
	readyCalls int
	leaveErrs  []error
	leaveCalls int
}

func (n *fakeNode) Ready() error {
	n.readyCalls++
	if n.readyCalls <= n.readyAfter {
		return errors.New("not a voter yet")
	}
	return nil
}

func (n *fakeNode) Leave() error {
	n.leaveCalls++
	if len(n.leaveErrs) == 0 {
		return nil
	}
	err := n.leaveErrs[0]
	n.leaveErrs = n.leaveErrs[1:]
	return err
}

func runFor(t *testing.T, api *fakeLifecycleAPI, node *fakeNode, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	opts := LifecycleOptions{Poll: time.Millisecond, LeaveTimeout: 20 * time.Millisecond, LeaveRetry: time.Millisecond, Logf: t.Logf}
	if err := RunLifecycle(ctx, api, node, opts); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunLifecycle: %v", err)
	}
}

func TestLifecycleLaunchHookWaitsForVoter(t *testing.T) {
	api := &fakeLifecycleAPI{states: []string{StatePendingWait}}
	node := &fakeNode{readyAfter: 3}
	runFor(t, api, node, 100*time.Millisecond)
	if node.readyCalls < 4 {
		t.Fatalf("Ready called %d times, want at least 4", node.readyCalls)
	}
	if len(api.completed) != 1 || api.completed[0] != LaunchHook {
		t.Fatalf("completed %v, want [%s] once", api.completed, LaunchHook)
	}
}

func TestLifecycleTerminateLeavesOnceThenContinues(t *testing.T) {
	api := &fakeLifecycleAPI{states: []string{"InService", StateTerminatingWait}}
	node := &fakeNode{leaveErrs: []error{errors.New("no leader"), nil}}
	runFor(t, api, node, 100*time.Millisecond)
	if node.leaveCalls != 2 {
		t.Fatalf("Leave called %d times, want 2 (one retry)", node.leaveCalls)
	}
	if len(api.completed) != 1 || api.completed[0] != TerminateHook {
		t.Fatalf("completed %v, want [%s] once", api.completed, TerminateHook)
	}
}

func TestLifecycleTerminateContinuesWhenLeaveNeverSucceeds(t *testing.T) {
	api := &fakeLifecycleAPI{states: []string{StateTerminatingWait}}
	node := &fakeNode{leaveErrs: []error{errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down")}}
	runFor(t, api, node, 150*time.Millisecond)
	if len(api.completed) != 1 || api.completed[0] != TerminateHook {
		t.Fatalf("completed %v, want [%s]: termination must continue after the leave timeout", api.completed, TerminateHook)
	}
}

func TestLifecycleSoleVoterContinuesWithoutRetry(t *testing.T) {
	api := &fakeLifecycleAPI{states: []string{StateTerminatingWait}}
	node := &fakeNode{leaveErrs: []error{ErrSoleVoter}}
	runFor(t, api, node, 50*time.Millisecond)
	if node.leaveCalls != 1 {
		t.Fatalf("Leave called %d times, want 1", node.leaveCalls)
	}
	if len(api.completed) != 1 {
		t.Fatalf("completed %v, want the terminate hook", api.completed)
	}
}

func TestLifecycleRetriesHookCompletion(t *testing.T) {
	api := &fakeLifecycleAPI{states: []string{StateTerminatingWait}, failHook: TerminateHook}
	node := &fakeNode{}
	runFor(t, api, node, 100*time.Millisecond)
	if node.leaveCalls != 1 {
		t.Fatalf("Leave called %d times, want 1: a failed hook call must not re-run the leave", node.leaveCalls)
	}
	if len(api.completed) != 1 {
		t.Fatalf("completed %v, want one successful terminate hook after a retry", api.completed)
	}
}

func TestLifecycleStopsOutsideGroup(t *testing.T) {
	api := &fakeLifecycleAPI{}
	err := RunLifecycle(context.Background(), api, &fakeNode{}, LifecycleOptions{Poll: time.Millisecond})
	if !errors.Is(err, ErrNotInGroup) {
		t.Fatalf("got %v, want ErrNotInGroup", err)
	}
}
