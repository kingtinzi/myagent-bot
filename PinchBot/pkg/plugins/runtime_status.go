package plugins

import (
	"os/exec"
	"strconv"
	"strings"
)

const (
	RuntimeStatusReady    = "ready"
	RuntimeStatusDegraded = "degraded"
	RuntimeStatusBlocked  = "blocked"
)

type RuntimeDependencyCheck struct {
	ID         string `json:"id"`
	OK         bool   `json:"ok"`
	Detail     string `json:"detail,omitempty"`
	ReasonCode string `json:"reason_code,omitempty"`
}

type RuntimeRepairAction struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Risk             string `json:"risk,omitempty"`
	RequiresApproval bool   `json:"requires_approval,omitempty"`
}

var runtimeProbe = probeRuntimeForPlugin

func probeRuntimeForPlugin(pluginID string) (status string, checks []RuntimeDependencyCheck, repairs []RuntimeRepairAction) {
	if !strings.EqualFold(strings.TrimSpace(pluginID), "lobster") {
		return "", nil, nil
	}

	nodeVersion, nodeOK := probeNodeVersion()
	checks = append(checks, RuntimeDependencyCheck{
		ID:         "node",
		OK:         nodeOK,
		Detail:     nodeVersion,
		ReasonCode: reasonWhenFalse(nodeOK, "NODE_NOT_FOUND"),
	})

	_, npmErr := exec.LookPath("npm")
	npmOK := npmErr == nil
	checks = append(checks, RuntimeDependencyCheck{
		ID:         "npm",
		OK:         npmOK,
		Detail:     detailForErr("npm command missing", npmErr),
		ReasonCode: reasonWhenErr(npmErr, "NPM_NOT_FOUND"),
	})

	_, lobsterErr := exec.LookPath("lobster")
	lobsterOK := lobsterErr == nil
	checks = append(checks, RuntimeDependencyCheck{
		ID:         "exe:lobster",
		OK:         lobsterOK,
		Detail:     detailForErr("lobster command missing", lobsterErr),
		ReasonCode: reasonWhenErr(lobsterErr, "BIN_NOT_FOUND"),
	})

	status = RuntimeStatusReady
	for _, c := range checks {
		if !c.OK {
			status = RuntimeStatusDegraded
			break
		}
	}

	if !lobsterOK {
		repairs = append(repairs,
			RuntimeRepairAction{
				ID:    "install_node_deps",
				Label: "安装扩展 Node 依赖",
				Risk:  "low",
			},
			RuntimeRepairAction{
				ID:               "install_bundled_cli",
				Label:            "安装内置 Lobster 运行时",
				Risk:             "medium",
				RequiresApproval: true,
			},
			RuntimeRepairAction{
				ID:    "set_env_path_hint",
				Label: "查看手动路径配置指引",
				Risk:  "low",
			},
		)
	}

	return status, checks, repairs
}

func probeNodeVersion() (string, bool) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return detailForErr("node command missing", err), false
	}
	out, err := exec.Command(nodePath, "--version").CombinedOutput()
	if err != nil {
		return detailForErr(strings.TrimSpace(string(out)), err), false
	}
	verRaw := strings.TrimSpace(string(out))
	if verRaw == "" {
		return "node --version returned empty output", false
	}
	if !nodeVersionAtLeast(verRaw, 18) {
		return verRaw, false
	}
	return verRaw, true
}

func nodeVersionAtLeast(raw string, minMajor int) bool {
	v := strings.TrimSpace(strings.TrimPrefix(raw, "v"))
	if v == "" {
		return false
	}
	majorPart := strings.SplitN(v, ".", 2)[0]
	major, err := strconv.Atoi(majorPart)
	if err != nil {
		return false
	}
	return major >= minMajor
}

func reasonWhenFalse(ok bool, reason string) string {
	if ok {
		return ""
	}
	return reason
}

func reasonWhenErr(err error, reason string) string {
	if err == nil {
		return ""
	}
	return reason
}

func detailForErr(base string, err error) string {
	base = strings.TrimSpace(base)
	if err == nil {
		return base
	}
	if base == "" {
		return err.Error()
	}
	return base + ": " + err.Error()
}

