// Package asg answers lifecycle hooks on the node's own Auto Scaling group.
package asg

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"

	"github.com/nullstone-modules/vault-cluster/internal/vaultcluster"
)

type Client struct {
	inner      *autoscaling.Client
	instanceID string
	group      string
}

// New talks about one instance, which is this node: the Raft node ID is the instance ID.
func New(instanceID string) (*Client, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("VAULT_RAFT_NODE_ID (the instance ID) is required")
	}
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("AWS credentials: %w", err)
	}
	return &Client{inner: autoscaling.NewFromConfig(cfg), instanceID: instanceID}, nil
}

// State is the instance's lifecycle state, e.g. Pending:Wait, InService, Terminating:Wait.
func (c *Client) State(ctx context.Context) (string, error) {
	out, err := c.inner.DescribeAutoScalingInstances(ctx, &autoscaling.DescribeAutoScalingInstancesInput{
		InstanceIds: []string{c.instanceID},
	})
	if err != nil {
		return "", fmt.Errorf("describe auto scaling instance %s: %w", c.instanceID, err)
	}
	if len(out.AutoScalingInstances) == 0 {
		return "", vaultcluster.ErrNotInGroup
	}
	inst := out.AutoScalingInstances[0]
	c.group = aws.ToString(inst.AutoScalingGroupName)
	return aws.ToString(inst.LifecycleState), nil
}

// Complete continues the named hook for this instance.
func (c *Client) Complete(ctx context.Context, hook string) error {
	if c.group == "" {
		if _, err := c.State(ctx); err != nil {
			return err
		}
	}
	_, err := c.inner.CompleteLifecycleAction(ctx, &autoscaling.CompleteLifecycleActionInput{
		AutoScalingGroupName:  aws.String(c.group),
		LifecycleHookName:     aws.String(hook),
		LifecycleActionResult: aws.String("CONTINUE"),
		InstanceId:            aws.String(c.instanceID),
	})
	if err != nil {
		return fmt.Errorf("complete lifecycle action %s: %w", hook, err)
	}
	return nil
}
