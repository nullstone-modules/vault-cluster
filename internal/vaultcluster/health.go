package vaultcluster

import (
	"fmt"
	"os"
)

func (c *Client) Health() error {
	st, err := c.API.Sys().SealStatus()
	if err != nil {
		return err
	}
	fmt.Printf("initialized %v\n", st.Initialized)
	fmt.Printf("sealed      %v\n", st.Sealed)
	if st.Sealed {
		return fmt.Errorf("vault is sealed")
	}
	if c.Cfg.Token != "" {
		if _, err := c.API.Logical().Read("sys/mounts/" + c.Cfg.KVMount + "/tune"); err != nil {
			fmt.Fprintf(os.Stderr, "kv mount: %v\n", err)
			return err
		}
		fmt.Printf("kv          %s/ (v2)\n", c.Cfg.KVMount)
	}
	fmt.Println("healthy")
	return nil
}
