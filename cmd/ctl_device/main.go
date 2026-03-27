package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/0xdevelop/ctl_device/config"
	"github.com/0xdevelop/ctl_device/internal/agent"
	"github.com/0xdevelop/ctl_device/internal/client"
	"github.com/0xdevelop/ctl_device/internal/event"
	"github.com/0xdevelop/ctl_device/internal/fileutil"
	"github.com/0xdevelop/ctl_device/internal/notify"
	"github.com/0xdevelop/ctl_device/internal/project"
	"github.com/0xdevelop/ctl_device/internal/recovery"
	"github.com/0xdevelop/ctl_device/internal/server"
	"github.com/0xdevelop/ctl_device/pkg/protocol"
)

func main() {
	var (
		connectAddr   string
		rootToken     string
		configFile    string
		jsonrpcPort   int
		mcpPort       int
		dashboardPort int
		grpcPort      int
		stateDir      string
	)

	rootCmd := &cobra.Command{
		Use:   "ctl_device",
		Short: "ctl_device - multi-agent task coordination server",
		Long:  "ctl_device is a task coordination system for multi-agent workflows via MCP protocol.\n\nRun without arguments to start in full mode. Use --connect to run as a client.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if connectAddr != "" {
				return runClientMode(connectAddr, rootToken, configFile)
			}
			return runServer(jsonrpcPort, mcpPort, dashboardPort, grpcPort, rootToken, stateDir, configFile)
		},
	}
	rootCmd.Flags().StringVarP(&connectAddr, "connect", "c", "", "Connect to full node address (enables client mode), e.g. http://192.168.1.100:3711")
	rootCmd.Flags().StringVarP(&rootToken, "token", "t", "", "Authentication token")
	rootCmd.Flags().StringVar(&configFile, "config", "", "Config file path")
	rootCmd.Flags().IntVar(&jsonrpcPort, "jsonrpc-port", 0, "JSON-RPC server port (default 3711)")
	rootCmd.Flags().IntVar(&mcpPort, "mcp-port", 0, "MCP SSE server port (default 3710)")
	rootCmd.Flags().IntVar(&dashboardPort, "dashboard-port", 0, "Dashboard port (default 3712)")
	rootCmd.Flags().IntVar(&grpcPort, "grpc-port", 0, "gRPC server port (default 3713)")
	rootCmd.Flags().StringVar(&stateDir, "state-dir", "", "State directory")

	// --- Deprecated: server subcommand ---
	var (
		svrJSONRPCPort   int
		svrMCPPort       int
		svrDashboardPort int
		svrGRPCPort      int
		svrToken         string
		svrStateDir      string
		svrConfigFile    string
	)
	serverCmd := &cobra.Command{
		Use:        "server",
		Short:      "[deprecated] Start the coordination server",
		Long:       "Deprecated: just run 'ctl_device' directly.",
		Deprecated: "just run 'ctl_device' directly.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(svrJSONRPCPort, svrMCPPort, svrDashboardPort, svrGRPCPort, svrToken, svrStateDir, svrConfigFile)
		},
	}
	serverCmd.Flags().IntVar(&svrJSONRPCPort, "jsonrpc-port", 0, "JSON-RPC server port (default 3711)")
	serverCmd.Flags().IntVar(&svrMCPPort, "mcp-port", 0, "MCP SSE server port (default 3710)")
	serverCmd.Flags().IntVar(&svrDashboardPort, "dashboard-port", 0, "Dashboard port (default 3712)")
	serverCmd.Flags().IntVar(&svrGRPCPort, "grpc-port", 0, "gRPC server port (default 3713)")
	serverCmd.Flags().StringVarP(&svrToken, "token", "t", "", "Authentication token")
	serverCmd.Flags().StringVar(&svrStateDir, "state-dir", "", "State directory")
	serverCmd.Flags().StringVarP(&svrConfigFile, "config", "c", "", "Config file path")

	// --- Top-level: mcp subcommand ---
	var (
		tlMCPServer string
		tlMCPToken  string
		tlMCPConfig string
	)
	tlMCPCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP stdio client (proxy to remote JSON-RPC server)",
		Long:  "Start MCP stdio client that proxies MCP requests to a remote JSON-RPC server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPClient(tlMCPServer, tlMCPToken, tlMCPConfig)
		},
	}
	tlMCPCmd.Flags().StringVarP(&tlMCPServer, "server", "s", "http://localhost:3711", "Remote JSON-RPC server URL")
	tlMCPCmd.Flags().StringVarP(&tlMCPToken, "token", "t", "", "Authentication token")
	tlMCPCmd.Flags().StringVarP(&tlMCPConfig, "config", "c", "", "Client config file path")

	// --- Top-level: status subcommand ---
	var (
		tlStatusServer  string
		tlStatusToken   string
		tlStatusAgent   string
		tlStatusProject string
		tlStatusConfig  string
	)
	tlStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show project/task status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(tlStatusServer, tlStatusToken, tlStatusAgent, tlStatusProject, tlStatusConfig)
		},
	}
	tlStatusCmd.Flags().StringVarP(&tlStatusServer, "server", "s", "http://localhost:3711", "Server URL")
	tlStatusCmd.Flags().StringVarP(&tlStatusToken, "token", "t", "", "Authentication token")
	tlStatusCmd.Flags().StringVarP(&tlStatusAgent, "agent", "a", "", "Agent ID")
	tlStatusCmd.Flags().StringVarP(&tlStatusProject, "project", "p", "", "Project filter")
	tlStatusCmd.Flags().StringVarP(&tlStatusConfig, "config", "c", "", "Client config file path")

	// --- Top-level: dispatch subcommand ---
	var (
		tlDispServer  string
		tlDispToken   string
		tlDispAgent   string
		tlDispProject string
		tlDispFile    string
		tlDispConfig  string
	)
	tlDispatchCmd := &cobra.Command{
		Use:   "dispatch",
		Short: "Dispatch a task to an agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDispatch(tlDispServer, tlDispToken, tlDispAgent, tlDispProject, tlDispFile, tlDispConfig)
		},
	}
	tlDispatchCmd.Flags().StringVarP(&tlDispServer, "server", "s", "http://localhost:3711", "Server URL")
	tlDispatchCmd.Flags().StringVarP(&tlDispToken, "token", "t", "", "Authentication token")
	tlDispatchCmd.Flags().StringVarP(&tlDispAgent, "agent", "a", "", "Agent ID")
	tlDispatchCmd.Flags().StringVarP(&tlDispProject, "project", "p", "", "Project name")
	tlDispatchCmd.Flags().StringVarP(&tlDispFile, "task-file", "f", "", "Task file to dispatch")
	tlDispatchCmd.Flags().StringVarP(&tlDispConfig, "config", "c", "", "Client config file path")
	_ = tlDispatchCmd.MarkFlagRequired("project")
	_ = tlDispatchCmd.MarkFlagRequired("task-file")

	// --- Top-level: logs subcommand ---
	var (
		tlLogsServer  string
		tlLogsToken   string
		tlLogsProject string
		tlLogsFollow  bool
		tlLogsConfig  string
	)
	tlLogsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Show server logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(tlLogsServer, tlLogsToken, tlLogsProject, tlLogsFollow, tlLogsConfig)
		},
	}
	tlLogsCmd.Flags().StringVarP(&tlLogsServer, "server", "s", "http://localhost:3711", "Server URL")
	tlLogsCmd.Flags().StringVarP(&tlLogsToken, "token", "t", "", "Authentication token")
	tlLogsCmd.Flags().StringVarP(&tlLogsProject, "project", "p", "", "Project filter")
	tlLogsCmd.Flags().BoolVarP(&tlLogsFollow, "follow", "f", false, "Follow logs (SSE)")
	tlLogsCmd.Flags().StringVarP(&tlLogsConfig, "config", "c", "", "Client config file path")

	// --- Deprecated: client subcommand (with sub-commands for backward compat) ---
	var (
		clMCPServer string
		clMCPToken  string
		clMCPConfig string
	)
	clMCPCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP stdio client (proxy to remote JSON-RPC server)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPClient(clMCPServer, clMCPToken, clMCPConfig)
		},
	}
	clMCPCmd.Flags().StringVarP(&clMCPServer, "server", "s", "http://localhost:3711", "Remote JSON-RPC server URL")
	clMCPCmd.Flags().StringVarP(&clMCPToken, "token", "t", "", "Authentication token")
	clMCPCmd.Flags().StringVarP(&clMCPConfig, "config", "c", "", "Client config file path")

	var (
		clStatusServer  string
		clStatusToken   string
		clStatusAgent   string
		clStatusProject string
		clStatusConfig  string
	)
	clStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show project/task status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(clStatusServer, clStatusToken, clStatusAgent, clStatusProject, clStatusConfig)
		},
	}
	clStatusCmd.Flags().StringVarP(&clStatusServer, "server", "s", "http://localhost:3711", "Server URL")
	clStatusCmd.Flags().StringVarP(&clStatusToken, "token", "t", "", "Authentication token")
	clStatusCmd.Flags().StringVarP(&clStatusAgent, "agent", "a", "", "Agent ID")
	clStatusCmd.Flags().StringVarP(&clStatusProject, "project", "p", "", "Project filter")
	clStatusCmd.Flags().StringVarP(&clStatusConfig, "config", "c", "", "Client config file path")

	var (
		clDispServer  string
		clDispToken   string
		clDispAgent   string
		clDispProject string
		clDispFile    string
		clDispConfig  string
	)
	clDispatchCmd := &cobra.Command{
		Use:   "dispatch",
		Short: "Dispatch a task to an agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDispatch(clDispServer, clDispToken, clDispAgent, clDispProject, clDispFile, clDispConfig)
		},
	}
	clDispatchCmd.Flags().StringVarP(&clDispServer, "server", "s", "http://localhost:3711", "Server URL")
	clDispatchCmd.Flags().StringVarP(&clDispToken, "token", "t", "", "Authentication token")
	clDispatchCmd.Flags().StringVarP(&clDispAgent, "agent", "a", "", "Agent ID")
	clDispatchCmd.Flags().StringVarP(&clDispProject, "project", "p", "", "Project name")
	clDispatchCmd.Flags().StringVarP(&clDispFile, "task-file", "f", "", "Task file to dispatch")
	clDispatchCmd.Flags().StringVarP(&clDispConfig, "config", "c", "", "Client config file path")
	_ = clDispatchCmd.MarkFlagRequired("project")
	_ = clDispatchCmd.MarkFlagRequired("task-file")

	var (
		clLogsServer  string
		clLogsToken   string
		clLogsProject string
		clLogsFollow  bool
		clLogsConfig  string
	)
	clLogsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Show server logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(clLogsServer, clLogsToken, clLogsProject, clLogsFollow, clLogsConfig)
		},
	}
	clLogsCmd.Flags().StringVarP(&clLogsServer, "server", "s", "http://localhost:3711", "Server URL")
	clLogsCmd.Flags().StringVarP(&clLogsToken, "token", "t", "", "Authentication token")
	clLogsCmd.Flags().StringVarP(&clLogsProject, "project", "p", "", "Project filter")
	clLogsCmd.Flags().BoolVarP(&clLogsFollow, "follow", "f", false, "Follow logs (SSE)")
	clLogsCmd.Flags().StringVarP(&clLogsConfig, "config", "c", "", "Client config file path")

	clientCmd := &cobra.Command{
		Use:        "client",
		Short:      "[deprecated] Client commands",
		Long:       "Deprecated: use 'ctl_device --connect <addr>' instead.",
		Deprecated: "use 'ctl_device --connect <addr>' instead.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	clientCmd.AddCommand(clMCPCmd, clStatusCmd, clDispatchCmd, clLogsCmd)

	rootCmd.AddCommand(serverCmd, clientCmd, tlMCPCmd, tlStatusCmd, tlDispatchCmd, tlLogsCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServer(jsonrpcPort, mcpPort, dashboardPort, grpcPort int, token, stateDir, configFile string) error {
	var cfg *config.ServerConfig
	var err error

	// Priority: CLI flag > env > beside-binary conf/config.yaml > default (auto-generate)
	resolvedConfig := configFile
	if resolvedConfig == "" {
		execPath, execErr := os.Executable()
		if execErr == nil {
			candidate := filepath.Join(filepath.Dir(execPath), "conf", "config.yaml")
			if _, statErr := os.Stat(candidate); statErr == nil {
				resolvedConfig = candidate
			}
		}
	}

	if resolvedConfig != "" {
		cfg, err = config.LoadServerConfig(resolvedConfig)
		if err != nil {
			return fmt.Errorf("failed to load config %s: %w", resolvedConfig, err)
		}
		fmt.Printf("Config loaded from: %s\n", resolvedConfig)
	} else {
		cfg = config.DefaultServerConfig()
		execPath, execErr := os.Executable()
		if execErr == nil {
			defaultPath := filepath.Join(filepath.Dir(execPath), "conf", "config.yaml")
			if writeErr := config.WriteDefaultConfig(defaultPath); writeErr == nil {
				fmt.Printf("Default config generated: %s\n", defaultPath)
			}
		}
	}

	// Override with environment variables
	if envToken := os.Getenv("CTL_DEVICE_TOKEN"); envToken != "" {
		cfg.Server.Token = envToken
	}
	if envAddr := os.Getenv("CTL_DEVICE_ADDR"); envAddr != "" {
		cfg.Server.Bind = envAddr
	}
	if envStateDir := os.Getenv("CTL_DEVICE_STATE_DIR"); envStateDir != "" {
		cfg.Server.StateDir = envStateDir
	}

	// Override with CLI arguments (highest priority, 0 = not set)
	if token != "" {
		cfg.Server.Token = token
	}
	if jsonrpcPort != 0 {
		cfg.Server.JSONRPCPort = jsonrpcPort
	}
	if mcpPort != 0 {
		cfg.Server.MCPPort = mcpPort
	}
	if dashboardPort != 0 {
		cfg.Server.DashboardPort = dashboardPort
	}
	if grpcPort != 0 {
		cfg.Server.GRPCPort = grpcPort
	}
	if stateDir != "" {
		cfg.Server.StateDir = stateDir
	}

	// Acquire full-mode lock to ensure only one full instance runs.
	lockPath := filepath.Join(cfg.Server.StateDir, "ctl_device.lock")
	if err := fileutil.AcquireLock(lockPath); err != nil {
		return fmt.Errorf("%s", err)
	}
	defer fileutil.ReleaseLock(lockPath)

	store, err := project.NewFileStore(cfg.Server.StateDir)
	if err != nil {
		return fmt.Errorf("failed to create file store: %w", err)
	}

	eventBus := event.NewBus()
	scheduler := project.NewScheduler(store, eventBus)

	registry, err := agent.NewRegistry(cfg.Server.StateDir)
	if err != nil {
		return fmt.Errorf("failed to create agent registry: %w", err)
	}
	manager, err := agent.NewManager(registry, store, eventBus)
	if err != nil {
		return fmt.Errorf("failed to create agent manager: %w", err)
	}

	jsonrpcServer, err := server.NewServer(
		fmt.Sprintf("%s:%d", cfg.Server.Bind, cfg.Server.JSONRPCPort),
		cfg.Server.Token,
		manager,
		scheduler,
		store,
		eventBus,
	)
	if err != nil {
		return fmt.Errorf("failed to create JSON-RPC server: %w", err)
	}

	if cfg.Server.TLS.Enabled {
		jsonrpcServer.SetTLSConfig(
			cfg.Server.TLS.Enabled,
			cfg.Server.TLS.CertFile,
			cfg.Server.TLS.KeyFile,
			cfg.Server.TLS.AutoTLS,
			cfg.Server.TLS.Domain,
		)
	}

	jsonrpcServer.SubscribeToEvents()

	notifier := notify.NewNotifier(cfg.Notify.Channel, cfg.Notify.Target)

	recoveryMgr := recovery.NewManager(scheduler, manager, notifier, eventBus)
	if err := recoveryMgr.OnServerStart(); err != nil {
		fmt.Fprintf(os.Stderr, "Recovery warning: %v\n", err)
	}

	mcpSSEServer := server.NewMCPSSEServer(
		fmt.Sprintf("%s:%d", cfg.Server.Bind, cfg.Server.MCPPort),
		scheduler,
		manager,
		store,
		eventBus,
	)

	dashboard := server.NewDashboard(
		fmt.Sprintf("%s:%d", cfg.Server.Bind, cfg.Server.DashboardPort),
		manager,
		scheduler,
		eventBus,
	)

	grpcServer := server.NewGRPCServer(
		fmt.Sprintf("%s:%d", cfg.Server.Bind, cfg.Server.GRPCPort),
		cfg.Server.Token,
		manager,
		scheduler,
		store,
		eventBus,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snapshotInterval := time.Duration(cfg.Server.SnapshotIntervalSecs) * time.Second
	scheduler.StartSnapshotLoop(ctx, snapshotInterval)
	scheduler.CheckTimeouts(ctx, func(msg string) {
		fmt.Fprintf(os.Stderr, "TIMEOUT: %s\n", msg)
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nShutting down server...")
		cancel()
		manager.Shutdown()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		jsonrpcServer.Shutdown(shutdownCtx)
		mcpSSEServer.Shutdown(shutdownCtx)
		dashboard.Shutdown(shutdownCtx)
		grpcServer.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Starting ctl_device server on %s:%d\n", cfg.Server.Bind, cfg.Server.JSONRPCPort)
	if cfg.Server.Token != "" {
		fmt.Printf("Token authentication enabled\n")
	}
	fmt.Printf("Dashboard available at http://%s:%d\n", cfg.Server.Bind, cfg.Server.DashboardPort)
	fmt.Printf("State directory: %s\n", store.Dir())
	fmt.Printf("MCP SSE server at http://%s:%d/sse\n", cfg.Server.Bind, cfg.Server.MCPPort)
	fmt.Printf("gRPC server at %s:%d\n", cfg.Server.Bind, cfg.Server.GRPCPort)

	recoveryMgr.Start(ctx)
	go func() {
		fmt.Fprintf(os.Stderr, "Starting MCP SSE on %s:%d...\n", cfg.Server.Bind, cfg.Server.MCPPort)
		if err := mcpSSEServer.Start(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "MCP SSE failed: %v\n", err)
		}
	}()
	go func() {
		fmt.Fprintf(os.Stderr, "Starting dashboard on %s:%d...\n", cfg.Server.Bind, cfg.Server.DashboardPort)
		if err := dashboard.Start(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Dashboard failed: %v\n", err)
		}
	}()
	go func() {
		fmt.Fprintf(os.Stderr, "Starting gRPC server on %s:%d...\n", cfg.Server.Bind, cfg.Server.GRPCPort)
		if err := grpcServer.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "gRPC server failed: %v\n", err)
		}
	}()

	if err := jsonrpcServer.Start(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}

func runClientMode(connectAddr, token, configFile string) error {
	// Load unified config if available, fall back to client config
	var agentID, role string
	var capabilities []string
	var serverURL, authToken string

	if configFile != "" {
		ucfg, err := config.LoadConfig(configFile)
		if err == nil {
			agentID = ucfg.Client.AgentID
			role = ucfg.Client.Role
			capabilities = ucfg.Client.Capabilities
			serverURL = ucfg.Connect
			authToken = ucfg.Server.Token
		}
	}

	// CLI overrides
	if connectAddr != "" {
		serverURL = connectAddr
	}
	if token != "" {
		authToken = token
	}

	// Fall back to legacy client config
	if serverURL == "" {
		cfg, err := config.LoadClientConfig(configFile)
		if err != nil {
			cfg = config.DefaultClientConfig()
		}
		config.ApplyClientConfigOverrides(cfg, connectAddr, token, "")
		serverURL = cfg.Server
		authToken = cfg.Token
		if agentID == "" {
			agentID = cfg.AgentID
		}
		if role == "" {
			role = cfg.Role
		}
		if len(capabilities) == 0 {
			capabilities = cfg.Capabilities
		}
	}

	// Env overrides
	if envServer := os.Getenv("CTL_DEVICE_SERVER"); envServer != "" {
		serverURL = envServer
	}
	if envToken := os.Getenv("CTL_DEVICE_TOKEN"); envToken != "" {
		authToken = envToken
	}
	if envAgent := os.Getenv("CTL_DEVICE_AGENT_ID"); envAgent != "" {
		agentID = envAgent
	}

	if serverURL == "" {
		return fmt.Errorf("no server address specified; use --connect <addr> or set connect in config")
	}

	// Default agent ID from hostname
	if agentID == "" {
		hostname, _ := os.Hostname()
		agentID = hostname
	}
	if role == "" {
		role = "executor"
	}

	c := client.NewClient(serverURL, authToken, agentID)

	// Register agent with the full node
	regResp, err := c.AgentRegister(&client.RegisterRequest{
		AgentID:      agentID,
		Role:         role,
		Capabilities: capabilities,
	})
	if err != nil {
		return fmt.Errorf("failed to register with %s: %w", serverURL, err)
	}

	fmt.Printf("Connected to %s as %s (role: %s)\n", serverURL, agentID, role)
	if regResp.PendingTasks != nil && len(regResp.PendingTasks) > 0 {
		fmt.Printf("  Pending tasks: %d\n", len(regResp.PendingTasks))
	}

	// Start heartbeat loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.Heartbeat(agentID); err != nil {
					fmt.Fprintf(os.Stderr, "Heartbeat failed: %v\n", err)
				}
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancel()
	fmt.Fprintln(os.Stderr, "\nClient disconnected.")
	return nil
}

func runStatus(serverURL, token, agentID, projectFilter, clientConfigPath string) error {
	var cfg *config.ClientConfig
	var err error

	if clientConfigPath != "" {
		cfg, err = config.LoadClientConfig(clientConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		cfg, err = config.LoadClientConfig("")
		if err != nil {
			cfg = config.DefaultClientConfig()
		}
	}

	config.ApplyClientConfigOverrides(cfg, serverURL, token, agentID)

	c := client.NewClient(cfg.Server, cfg.Token, cfg.AgentID)

	resp, err := c.ProjectList()
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	if len(resp.Projects) == 0 {
		fmt.Println("No projects registered")
		return nil
	}

	for _, proj := range resp.Projects {
		if projectFilter != "" && proj.Name != projectFilter {
			continue
		}

		fmt.Printf("\nProject: %s\n", proj.Name)
		fmt.Printf("  Directory: %s\n", proj.Dir)
		fmt.Printf("  Tech: %s\n", proj.Tech)
		fmt.Printf("  Executor: %s\n", proj.Executor)
		fmt.Printf("  Timeout: %d minutes\n", proj.TimeoutMinutes)

		tasks := resp.Tasks[proj.Name]
		if len(tasks) == 0 {
			fmt.Printf("  Tasks: none\n")
			continue
		}

		fmt.Printf("  Tasks:\n")
		for _, task := range tasks {
			fmt.Printf("    - [%s] %s: %s\n", task.Status, task.Num, task.Name)
			if task.AssignedTo != "" {
				fmt.Printf("      Assigned to: %s\n", task.AssignedTo)
			}
			if task.Commit != "" {
				fmt.Printf("      Commit: %s\n", task.Commit)
			}
		}
	}

	return nil
}

func runDispatch(serverURL, token, agentID, projectName, taskFile, clientConfigPath string) error {
	var cfg *config.ClientConfig
	var err error

	if clientConfigPath != "" {
		cfg, err = config.LoadClientConfig(clientConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		cfg, err = config.LoadClientConfig("")
		if err != nil {
			cfg = config.DefaultClientConfig()
		}
	}

	config.ApplyClientConfigOverrides(cfg, serverURL, token, agentID)

	c := client.NewClient(cfg.Server, cfg.Token, cfg.AgentID)

	data, err := os.ReadFile(taskFile)
	if err != nil {
		return fmt.Errorf("failed to read task file: %w", err)
	}

	var task interface{}
	if err := json.Unmarshal(data, &task); err != nil {
		return fmt.Errorf("failed to parse task file: %w", err)
	}

	if err := c.TaskDispatch(projectName, task); err != nil {
		return fmt.Errorf("failed to dispatch task: %w", err)
	}

	fmt.Printf("Task dispatched to project %s\n", projectName)
	return nil
}

func runLogs(serverURL, token, projectFilter string, follow bool, clientConfigPath string) error {
	var cfg *config.ClientConfig
	var err error

	if clientConfigPath != "" {
		cfg, err = config.LoadClientConfig(clientConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		cfg, err = config.LoadClientConfig("")
		if err != nil {
			cfg = config.DefaultClientConfig()
		}
	}

	config.ApplyClientConfigOverrides(cfg, serverURL, token, "")

	c := client.NewClient(cfg.Server, cfg.Token, cfg.AgentID)

	if !follow {
		return fmt.Errorf("logs without --follow not yet implemented")
	}

	eventCh, errCh, err := c.SubscribeEvents(projectFilter)
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	for {
		select {
		case event := <-eventCh:
			data, _ := json.MarshalIndent(event, "", "  ")
			fmt.Printf("%s\n", string(data))
		case err := <-errCh:
			return fmt.Errorf("event stream error: %w", err)
		}
	}
}

func runMCPClient(serverURL, token, clientConfigPath string) error {
	var cfg *config.ClientConfig
	var err error

	if clientConfigPath != "" {
		cfg, err = config.LoadClientConfig(clientConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		cfg, err = config.LoadClientConfig("")
		if err != nil {
			cfg = config.DefaultClientConfig()
		}
	}

	config.ApplyClientConfigOverrides(cfg, serverURL, token, "")

	c := client.NewClient(cfg.Server, cfg.Token, cfg.AgentID)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var mcpReq map[string]interface{}
		if err := json.Unmarshal([]byte(line), &mcpReq); err != nil {
			sendMCPError(nil, -32700, "Parse error: "+err.Error())
			continue
		}

		if mcpReq["jsonrpc"] != "2.0" {
			sendMCPError(mcpReq["id"], -32600, "Invalid Request")
			continue
		}

		method, _ := mcpReq["method"].(string)
		params := mcpReq["params"]

		result, err := handleMCPMethod(method, params, c)
		if err != nil {
			sendMCPError(mcpReq["id"], -32000, err.Error())
			continue
		}

		sendMCPResponse(mcpReq["id"], result)
	}

	return scanner.Err()
}

func handleMCPMethod(method string, params interface{}, c *client.Client) (interface{}, error) {
	switch method {
	case "initialize":
		return map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "ctl_device",
				"version": "0.1.0",
			},
		}, nil

	case "notifications/initialized":
		return nil, nil

	case "tools/list":
		tools := []map[string]interface{}{
			toolToMap(protocol.ToolTaskGet),
			toolToMap(protocol.ToolTaskComplete),
			toolToMap(protocol.ToolTaskBlock),
			toolToMap(protocol.ToolTaskStatus),
			toolToMap(protocol.ToolProjectRegister),
			toolToMap(protocol.ToolProjectList),
			toolToMap(protocol.ToolTaskDispatch),
			toolToMap(protocol.ToolTaskAdvance),
			toolToMap(protocol.ToolAgentList),
		}
		return map[string]interface{}{
			"tools": tools,
		}, nil

	case "tools/call":
		paramsMap, ok := params.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid params")
		}

		name, _ := paramsMap["name"].(string)
		arguments, _ := paramsMap["arguments"].(map[string]interface{})

		return handleToolCall(name, arguments, c)

	default:
		return nil, fmt.Errorf("method not found: %s", method)
	}
}

func toolToMap(tool protocol.MCPToolSchema) map[string]interface{} {
	return map[string]interface{}{
		"name":        tool.Name,
		"description": tool.Description,
		"inputSchema": tool.InputSchema,
	}
}

func handleToolCall(name string, args map[string]interface{}, c *client.Client) (interface{}, error) {
	switch name {
	case "task_get":
		projectName, _ := args["project"].(string)
		if projectName == "" {
			return nil, fmt.Errorf("missing project")
		}

		resp, err := c.TaskGet(projectName)
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf(`{"status": "%s", "task": %v}`, resp.Status, resp.Task),
				},
			},
		}, nil

	case "task_complete":
		projectName, _ := args["project"].(string)
		taskNum, _ := args["task_num"].(string)
		summary, _ := args["summary"].(string)
		commit, _ := args["commit"].(string)
		testOutput, _ := args["test_output"].(string)
		issues, _ := args["issues"].(string)

		report := &client.CompleteReport{
			Project:    projectName,
			TaskNum:    taskNum,
			Summary:    summary,
			Commit:     commit,
			TestOutput: testOutput,
			Issues:     issues,
		}

		if err := c.TaskComplete(report); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": `{"status": "ok"}`},
			},
		}, nil

	case "task_block":
		projectName, _ := args["project"].(string)
		taskNum, _ := args["task_num"].(string)
		reason, _ := args["reason"].(string)
		details, _ := args["details"].(string)

		report := &client.BlockReport{
			Project: projectName,
			TaskNum: taskNum,
			Reason:  reason,
			Details: details,
		}

		if err := c.TaskBlock(report); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": `{"status": "ok"}`},
			},
		}, nil

	case "task_status":
		projectName, _ := args["project"].(string)
		taskNum, _ := args["task_num"].(string)
		status, _ := args["status"].(string)

		if err := c.TaskStatus(projectName, taskNum, status); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": `{"status": "ok"}`},
			},
		}, nil

	case "project_register":
		req := &client.ProjectRegisterRequest{
			Name:           getString(args, "name"),
			Dir:            getString(args, "dir"),
			Tech:           getString(args, "tech"),
			TestCmd:        getString(args, "test_cmd"),
			Executor:       getString(args, "executor"),
			TimeoutMinutes: getInt(args, "timeout_minutes"),
			NotifyChannel:  getString(args, "notify_channel"),
			NotifyTarget:   getString(args, "notify_target"),
		}

		if err := c.ProjectRegister(req); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": `{"status": "ok"}`},
			},
		}, nil

	case "project_list":
		resp, err := c.ProjectList()
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf(`{"projects": %v, "tasks": %v}`, resp.Projects, resp.Tasks)},
			},
		}, nil

	case "task_dispatch":
		projectName, _ := args["project"].(string)
		task, _ := args["task"].(map[string]interface{})

		if err := c.TaskDispatch(projectName, task); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": `{"status": "ok"}`},
			},
		}, nil

	case "task_advance":
		projectName, _ := args["project"].(string)

		if err := c.TaskAdvance(projectName); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": `{"status": "ok"}`},
			},
		}, nil

	case "agent_list":
		resp, err := c.AgentList()
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf(`{"agents": %v}`, resp.Agents)},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func getString(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func getInt(args map[string]interface{}, key string) int {
	if v, ok := args[key].(int); ok {
		return v
	}
	return 0
}

func sendMCPResponse(id interface{}, result interface{}) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", data)
}

func sendMCPError(id interface{}, code int, message string) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", data)
}
