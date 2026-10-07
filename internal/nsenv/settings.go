package nsenv

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Settings struct {
	Addr          string
	TLSServerName string
	// SharedEnvs is the cluster's shared output: tenants are scoped per env and vault-utils needs --env.
	SharedEnvs bool
	// Note explains a fallback, for stderr.
	Note string
}

// sharedOutput reads the cluster's shared output, which older clusters lack (false).
func sharedOutput(out map[string]any) bool {
	switch v := out["shared"].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return false
}

// SettingsFromOutputs prefers the user-facing address. internal selects the private name (vault_addr), which
// needs the user-facing name for TLS verification when the load balancer terminates TLS.
// Clusters older than the vault_addr outputs fall back to fqdn + port; they use TLS exactly when user_fqdn is set.
func SettingsFromOutputs(ws Workspace, internal bool) (Settings, error) {
	out := ws.Outputs
	str := func(k string) string {
		v, _ := out[k].(string)
		return strings.TrimSpace(v)
	}
	userFQDN, vaultFQDN, port := str("user_fqdn"), str("vault_fqdn"), str("vault_api_port")
	if port == "" {
		port = "8200"
	}
	scheme := "http"
	if userFQDN != "" {
		scheme = "https"
	}
	internalAddr := str("vault_addr")
	if internalAddr == "" && vaultFQDN != "" {
		internalAddr = fmt.Sprintf("%s://%s:%s", scheme, vaultFQDN, port)
	}
	userAddr := str("user_vault_addr")
	if userAddr == "" && userFQDN != "" {
		userAddr = fmt.Sprintf("https://%s:%s", userFQDN, port)
	}
	if internalAddr == "" && userAddr == "" {
		return Settings{}, fmt.Errorf("workspace has no vault_addr, user_vault_addr, or vault_fqdn output; has it been applied?")
	}
	shared := sharedOutput(out)
	if !internal && userAddr != "" {
		return Settings{Addr: userAddr, SharedEnvs: shared}, nil
	}
	s := Settings{Addr: internalAddr, SharedEnvs: shared}
	if !internal {
		s.Note = "no subdomain is connected to this cluster; using " + vaultFQDN + ", which resolves only inside the cluster network"
	}
	if strings.HasPrefix(internalAddr, "https://") {
		s.TLSServerName = str("tls_server_name")
		if s.TLSServerName == "" {
			s.TLSServerName = userFQDN
		}
	}
	return s, nil
}

func Render(shell string, s Settings) (string, error) {
	var b strings.Builder
	set := func(k, v string) {
		switch shell {
		case "fish":
			fmt.Fprintf(&b, "set -gx %s %s;\n", k, quoteFish(v))
		case "powershell":
			fmt.Fprintf(&b, "$env:%s = %s\n", k, quotePowerShell(v))
		default:
			fmt.Fprintf(&b, "export %s=%s\n", k, quotePOSIX(v))
		}
	}
	unset := func(k string) {
		switch shell {
		case "fish":
			fmt.Fprintf(&b, "set -e %s;\n", k)
		case "powershell":
			fmt.Fprintf(&b, "Remove-Item Env:%s -ErrorAction SilentlyContinue\n", k)
		default:
			fmt.Fprintf(&b, "unset %s\n", k)
		}
	}
	switch shell {
	case "bash", "zsh", "fish", "powershell":
	default:
		return "", fmt.Errorf("unknown shell %q (bash, zsh, fish, powershell)", shell)
	}
	set("VAULT_ADDR", s.Addr)
	if s.TLSServerName != "" {
		set("VAULT_TLS_SERVER_NAME", s.TLSServerName)
	} else {
		unset("VAULT_TLS_SERVER_NAME")
	}
	if s.SharedEnvs {
		set("SHARED_ENVS", "true")
	} else {
		unset("SHARED_ENVS")
	}
	return b.String(), nil
}

func quotePOSIX(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func quoteFish(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(v) + "'"
}

func quotePowerShell(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// CheckReachable probes sys/health. Any HTTP response counts; only network failures are errors.
func CheckReachable(ctx context.Context, s Settings, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.Addr, "/")+"/v1/sys/health", nil)
	if err != nil {
		return err
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{ServerName: s.TLSServerName, MinVersion: tls.VersionTLS12}}
	defer tr.CloseIdleConnections()
	res, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}
