package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

var tmuxSessionName = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return sameOrigin(r) },
}

type sshConnectionConfig struct {
	machineID string
	address   string
	client    *ssh.ClientConfig
	agentConn net.Conn
}

type renameSessionRequest struct {
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
}

type consoleSession struct {
	Name    string `json:"name"`
	Preview string `json:"preview"`
}

type terminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

func loadSSHConnectionConfig(cfg sshConfig) (*sshConnectionConfig, error) {
	if cfg.MachineID == "" || cfg.Host == "" || cfg.Port < 1 || cfg.User == "" || cfg.KnownHostsPath == "" || (cfg.PrivateKeyPath == "" && cfg.AgentSocket == "") {
		return nil, fmt.Errorf("ssh machine_id, host, port, user, known_hosts_path, and either private_key_path or agent_socket must be configured")
	}
	authMethods := make([]ssh.AuthMethod, 0, 2)
	var err error
	if cfg.PrivateKeyPath != "" {
		privateKey, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read SSH private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("parse SSH private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	var agentConn net.Conn
	if cfg.AgentSocket != "" {
		agentConn, err = net.Dial("unix", cfg.AgentSocket)
		if err != nil {
			return nil, fmt.Errorf("connect SSH agent: %w", err)
		}
		agentClient := agent.NewClient(agentConn)
		if _, err := agentClient.Signers(); err != nil {
			_ = agentConn.Close()
			return nil, fmt.Errorf("load SSH agent identities: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
	}
	hostKeyCallback, err := knownhosts.New(cfg.KnownHostsPath)
	if err != nil {
		if agentConn != nil {
			_ = agentConn.Close()
		}
		return nil, fmt.Errorf("load SSH known hosts: %w", err)
	}
	hostKeyAlgorithms := cfg.HostKeyAlgorithms
	if len(hostKeyAlgorithms) == 0 {
		hostKeyAlgorithms = []string{ssh.KeyAlgoED25519}
	}
	return &sshConnectionConfig{
		machineID: cfg.MachineID,
		address:   net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port)),
		agentConn: agentConn,
		client: &ssh.ClientConfig{
			User:              cfg.User,
			Auth:              authMethods,
			HostKeyCallback:   hostKeyCallback,
			HostKeyAlgorithms: hostKeyAlgorithms,
			Timeout:           10 * time.Second,
		},
	}, nil
}

func (c *sshConnectionConfig) close() error {
	if c.agentConn != nil {
		return c.agentConn.Close()
	}
	return nil
}

func (a *app) authorizeConsole(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := a.currentUserID(r)
	if !ok {
		log.Printf("console access denied: unauthenticated")
		http.Error(w, "login required", http.StatusUnauthorized)
		return 0, false
	}
	machineID := r.URL.Query().Get("machine")
	if machineID == "" {
		machineID = a.ssh.machineID
	}
	if machineID != a.ssh.machineID {
		log.Printf("console access denied: user_id=%d machine_id=%s", userID, machineID)
		http.Error(w, "unknown machine", http.StatusBadRequest)
		return 0, false
	}
	if _, allowed := a.consolePermissions[machineID][userID]; !allowed {
		log.Printf("console access denied: user_id=%d machine_id=%s", userID, machineID)
		http.Error(w, "console access denied", http.StatusForbidden)
		return 0, false
	}
	return userID, true
}

func (a *app) connectSSH(userID int64) (*ssh.Client, error) {
	log.Printf("SSH connection initiated: user_id=%d machine_id=%s", userID, a.ssh.machineID)
	connection, err := net.DialTimeout("tcp", a.ssh.address, 10*time.Second)
	if err != nil {
		return nil, err
	}
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, a.ssh.address, a.ssh.client)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	return ssh.NewClient(clientConnection, channels, requests), nil
}

func (a *app) consoleSessionsHandler(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	userID, ok := a.authorizeConsole(w, r)
	if !ok {
		return
	}
	client, err := a.connectSSH(userID)
	if err != nil {
		log.Printf("SSH connection failed: user_id=%d machine_id=%s error=%v", userID, a.ssh.machineID, err)
		http.Error(w, "could not connect to configured console", http.StatusBadGateway)
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		http.Error(w, "could not open console", http.StatusBadGateway)
		return
	}
	output, err := session.CombinedOutput("tmux list-sessions -F '#S'")
	_ = session.Close()
	if err != nil {
		if exitError, isExit := err.(*ssh.ExitError); isExit && exitError.ExitStatus() == 1 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]consoleSession{})
			return
		}
		log.Printf("tmux session listing failed: user_id=%d error=%v", userID, err)
		http.Error(w, "could not list tmux sessions", http.StatusBadGateway)
		return
	}

	sessions := make([]consoleSession, 0)
	for _, name := range strings.Fields(string(output)) {
		if tmuxSessionName.MatchString(name) {
			sessions = append(sessions, consoleSession{Name: name})
		}
	}
	var previews sync.WaitGroup
	for index := range sessions {
		previews.Add(1)
		go func(index int) {
			defer previews.Done()
			sessions[index].Preview = captureTmuxPreview(client, sessions[index].Name)
		}(index)
	}
	previews.Wait()
	log.Printf("console access permitted: user_id=%d machine_id=%s", userID, a.ssh.machineID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sessions)
}

func (a *app) renameConsoleSessionHandler(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	userID, ok := a.authorizeConsole(w, r)
	if !ok {
		return
	}
	var input renameSessionRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || !tmuxSessionName.MatchString(input.OldName) || !tmuxSessionName.MatchString(input.NewName) {
		http.Error(w, "valid tmux session names are required", http.StatusBadRequest)
		return
	}
	client, err := a.connectSSH(userID)
	if err != nil {
		log.Printf("SSH connection failed: user_id=%d machine_id=%s error=%v", userID, a.ssh.machineID, err)
		http.Error(w, "could not connect to configured console", http.StatusBadGateway)
		return
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		http.Error(w, "could not open console", http.StatusBadGateway)
		return
	}
	defer session.Close()
	if _, err := session.CombinedOutput(fmt.Sprintf("tmux rename-session -t %s %s", input.OldName, input.NewName)); err != nil {
		log.Printf("tmux rename failed: user_id=%d error=%v", userID, err)
		http.Error(w, "could not rename tmux session", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func captureTmuxPreview(client *ssh.Client, name string) string {
	session, err := client.NewSession()
	if err != nil {
		return ""
	}
	defer session.Close()
	output, err := session.Output(fmt.Sprintf("tmux capture-pane -p -e -t %s:0", name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (a *app) consoleTerminalHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.authorizeConsole(w, r)
	if !ok {
		return
	}
	sessionName := r.URL.Query().Get("session")
	if sessionName != "" && !tmuxSessionName.MatchString(sessionName) {
		http.Error(w, "invalid tmux session", http.StatusBadRequest)
		return
	}
	connection, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	client, err := a.connectSSH(userID)
	if err != nil {
		log.Printf("SSH connection failed: user_id=%d machine_id=%s error=%v", userID, a.ssh.machineID, err)
		_ = connection.WriteMessage(websocket.TextMessage, []byte("Unable to connect to configured console."))
		return
	}
	defer client.Close()
	sshSession, err := client.NewSession()
	if err != nil {
		_ = connection.WriteMessage(websocket.TextMessage, []byte("Unable to open console."))
		return
	}
	defer sshSession.Close()

	outputReader, outputWriter := io.Pipe()
	sshSession.Stdout = outputWriter
	sshSession.Stderr = outputWriter
	input, err := sshSession.StdinPipe()
	if err != nil || sshSession.RequestPty("xterm-256color", 34, 120, ssh.TerminalModes{}) != nil {
		_ = connection.WriteMessage(websocket.TextMessage, []byte("Unable to start console."))
		return
	}
	command := "tmux new-session -A -s web"
	if sessionName != "" {
		command = "tmux attach-session -t " + sessionName
	}
	if err := sshSession.Start(command); err != nil {
		log.Printf("tmux start failed: user_id=%d error=%v", userID, err)
		_ = connection.WriteMessage(websocket.TextMessage, []byte("Unable to start tmux."))
		return
	}
	log.Printf("tmux session attached: user_id=%d machine_id=%s session=%s", userID, a.ssh.machineID, sessionName)
	go func() { _ = sshSession.Wait(); _ = outputWriter.Close() }()
	go copyTerminalOutput(connection, outputReader)

	for {
		_, payload, readErr := connection.ReadMessage()
		if readErr != nil {
			return
		}
		var message terminalMessage
		if json.Unmarshal(payload, &message) != nil {
			continue
		}
		switch message.Type {
		case "input":
			if _, err := io.WriteString(input, message.Data); err != nil {
				return
			}
		case "resize":
			if message.Cols > 0 && message.Rows > 0 {
				_ = sshSession.WindowChange(message.Rows, message.Cols)
			}
		}
	}
}

func copyTerminalOutput(connection *websocket.Conn, output io.ReadCloser) {
	defer output.Close()
	buffer := make([]byte, 4096)
	for {
		count, err := output.Read(buffer)
		if count > 0 && connection.WriteMessage(websocket.BinaryMessage, buffer[:count]) != nil {
			return
		}
		if err != nil {
			return
		}
	}
}
