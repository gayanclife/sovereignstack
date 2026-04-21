sovereignstack/
├── cmd/                # CLI Command logic (Cobra)
│   ├── root.go         # The main 'sov' command
│   ├── init.go         # 'sov init' - Hardware check
│   └── deploy.go       # 'sov deploy' - Container logic
├── internal/           # Private library code (The "Engine")
│   ├── hardware/       # CPU, RAM, GPU detection logic
│   ├── engine/         # Docker/vLLM orchestration
│   └── tunnel/         # Tailscale/Networking logic
├── pkg/                # Publicly sharable logic (Optional)
├── main.go             # Entry point
└── go.mod              # Go dependencies
