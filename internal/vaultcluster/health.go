package vaultcluster

import (
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/vault/api"
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
		_, err := c.API.Logical().Read("sys/mounts/" + c.Cfg.KVMount + "/tune")
		var re *api.ResponseError
		switch {
		case errors.As(err, &re) && re.StatusCode == 403:
			// Tenant admins cannot read mount config.
			fmt.Printf("kv          not visible to this token\n")
		case err != nil:
			fmt.Fprintf(os.Stderr, "kv mount: %v\n", err)
			return err
		default:
			fmt.Printf("kv          %s/ (v2)\n", c.Cfg.KVMount)
		}
	}
	fmt.Println("healthy")
	return nil
}
