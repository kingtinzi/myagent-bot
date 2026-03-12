package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/sipeed/pinchbot/pkg/config"
)

// gatewayLogs stores captured stdout/stderr from the gateway process launched by the launcher.
var gatewayLogs = NewLogBuffer(200)

// RegisterProcessAPI registers endpoints to start, stop and check status of the PinchBot gateway.
func RegisterProcessAPI(mux *http.ServeMux, absPath string) {
	mux.HandleFunc("GET /api/process/status", func(w http.ResponseWriter, r *http.Request) {
		handleStatusGateway(w, r, absPath)
	})
	mux.HandleFunc("POST /api/process/start", handleStartGateway)
	mux.HandleFunc("POST /api/process/stop", handleStopGateway)
	// 供 launcher-chat 推送网关 stdout/stderr，使配置页能显示「由 launcher-chat 启动」的网关日志
	mux.HandleFunc("POST /api/process/logs/start", handleLogsStart)
	mux.HandleFunc("POST /api/process/logs", handleLogsAppend)
}

func handleLogsStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gatewayLogs.Reset()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleLogsAppend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Lines []string `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	for _, line := range body.Lines {
		if line != "" {
			gatewayLogs.Append(line)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// checkGatewayRunning 检测默认地址 127.0.0.1:18790 是否有网关在运行
func checkGatewayRunning() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:18790/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func handleStartGateway(w http.ResponseWriter, r *http.Request) {
	// 若网关已在运行，直接返回成功，避免重复启动导致端口冲突
	if checkGatewayRunning() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":         "ok",
			"already_running": true,
		})
		return
	}

	// Locate PinchBot executable: same directory as current executable (PinchBot-launcher)
	execPath := ""
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "PinchBot")
		if runtime.GOOS == "windows" {
			candidate += ".exe"
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			execPath = candidate
		}
	}
	if execPath == "" {
		log.Printf("PinchBot executable not found next to launcher (expected same dir as PinchBot-launcher)\n")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "PinchBot executable not found. Please place PinchBot.exe in the same directory as PinchBot-launcher.",
		})
		return
	}

	cmd := exec.Command(execPath, "gateway")
	cmd.Dir = filepath.Dir(execPath) // 与 launcher-chat 一致：网关工作目录为 exe 所在目录，便于找到 .pinchbot

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to create stdout pipe: %v\n", err)
		http.Error(w, fmt.Sprintf("Failed to start gateway: %v", err), http.StatusInternalServerError)
		return
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Failed to create stderr pipe: %v\n", err)
		http.Error(w, fmt.Sprintf("Failed to start gateway: %v", err), http.StatusInternalServerError)
		return
	}

	// Clear old logs and increment runID before starting
	gatewayLogs.Reset()

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start PinchBot gateway: %v\n", err)
		http.Error(w, fmt.Sprintf("Failed to start gateway: %v", err), http.StatusInternalServerError)
		return
	}

	// Read stdout and stderr into the log buffer
	go scanPipe(stdoutPipe, gatewayLogs)
	go scanPipe(stderrPipe, gatewayLogs)

	// Wait for the process to exit in the background to avoid zombies
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("Gateway process exited: %v\n", err)
		}
	}()

	log.Printf("Started PinchBot gateway (PID: %d) from %s\n", cmd.Process.Pid, execPath)
	gatewayLogs.Append(fmt.Sprintf("[launcher] Started gateway (PID: %d) from %s", cmd.Process.Pid, execPath))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"pid":    cmd.Process.Pid,
	})
}

// scanPipe reads lines from r and appends them to buf. It returns when r reaches EOF.
func scanPipe(r io.Reader, buf *LogBuffer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB per line

	for scanner.Scan() {
		buf.Append(scanner.Text())
	}
}

func handleStopGateway(w http.ResponseWriter, r *http.Request) {
	var err error
	if runtime.GOOS == "windows" {
		// Kill via taskkill finding PinchBot.exe (though it might kill this config tool if it's named PinchBot-launcher.exe...? No, /IM does exact match usually, but just to be safe let's stop exactly PinchBot.exe)
		// Alternatively, we use powershell to kill processes with commandline containing 'gateway'
		psCmd := `Get-WmiObject Win32_Process | Where-Object { $_.CommandLine -match 'PinchBot.*gateway' } | ForEach-Object { Stop-Process $_.ProcessId -Force }`
		err = exec.Command("powershell", "-Command", psCmd).Run()
	} else {
		// Linux/macOS
		err = exec.Command("pkill", "-f", "PinchBot gateway").Run()
	}

	if err != nil {
		log.Printf("Warning: Failed to stop gateway (perhaps not running?): %v\n", err)
		// We still return 200 OK because pkill returns an error if no process was found
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", // or "not_found"
			"msg":    "Stop command executed, but returned error (process might not be running).",
			"error":  err.Error(),
		})
		return
	}

	log.Printf("Stopped PinchBot gateway processes.\n")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func handleStatusGateway(w http.ResponseWriter, r *http.Request, absPath string) {
	cfg, cfgErr := config.LoadConfig(absPath)
	host := "127.0.0.1"
	port := 18790
	if cfgErr == nil && cfg != nil {
		if cfg.Gateway.Host != "" && cfg.Gateway.Host != "0.0.0.0" {
			host = cfg.Gateway.Host
		}
		if cfg.Gateway.Port != 0 {
			port = cfg.Gateway.Port
		}
	}

	url := fmt.Sprintf("http://%s/health", net.JoinHostPort(host, strconv.Itoa(port)))
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)

	// Build the response data map
	data := map[string]any{}

	if err != nil {
		data["process_status"] = "stopped"
		data["error"] = err.Error()
	} else {
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			data["process_status"] = "error"
			data["status_code"] = resp.StatusCode
		} else {
			var healthData map[string]any
			if decErr := json.NewDecoder(resp.Body).Decode(&healthData); decErr != nil {
				data["process_status"] = "error"
				data["error"] = "invalid response from gateway"
			} else {
				// Gateway is running and responded properly — merge health data
				for k, v := range healthData {
					data[k] = v
				}
				data["process_status"] = "running"
			}
		}
	}

	// Append log data from the buffer
	appendLogData(r, data)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// appendLogData reads log_offset and log_run_id query params from the request and
// populates the response data map with incremental log lines.
func appendLogData(r *http.Request, data map[string]any) {
	clientOffset := 0
	clientRunID := -1

	if v := r.URL.Query().Get("log_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			clientOffset = n
		}
	}

	if v := r.URL.Query().Get("log_run_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			clientRunID = n
		}
	}

	runID := gatewayLogs.RunID()

	// If runID is 0 (never reset = never launched from this launcher), report no source
	if runID == 0 {
		data["logs"] = []string{}
		data["log_total"] = 0
		data["log_run_id"] = 0
		data["log_source"] = "none"
		return
	}

	// If the client's runID doesn't match, send all buffered lines (gateway restarted)
	offset := clientOffset
	if clientRunID != runID {
		offset = 0
	}

	lines, total, runID := gatewayLogs.LinesSince(offset)
	if lines == nil {
		lines = []string{}
	}

	data["logs"] = lines
	data["log_total"] = total
	data["log_run_id"] = runID
	data["log_source"] = "launcher"
}
