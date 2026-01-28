package console

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	// ANSI color codes
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"

	checkMark = "\u2713"
	crossMark = "\u2717"
	bullet    = "\u2022"
)

// Console handles formatted output to the terminal
type Console struct {
	quiet     bool
	noColor   bool
	mu        sync.Mutex
	startTime time.Time
}

// New creates a new Console
func New(quiet, noColor bool) *Console {
	return &Console{
		quiet:     quiet,
		noColor:   noColor,
		startTime: time.Now(),
	}
}

// PrintBanner prints the startup banner
func (c *Console) PrintBanner(version string) {
	if c.quiet {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cyan := c.color(colorCyan)
	reset := c.color(colorReset)

	fmt.Println()
	fmt.Printf("%s╭─────────────────────────────────────────────────────────────╮%s\n", cyan, reset)
	fmt.Printf("%s│%s  tr-engine %-48s %s│%s\n", cyan, reset, "v"+version, cyan, reset)
	fmt.Printf("%s│%s  Real-time trunk-recorder data aggregation engine           %s│%s\n", cyan, reset, cyan, reset)
	fmt.Printf("%s╰─────────────────────────────────────────────────────────────╯%s\n", cyan, reset)
	fmt.Println()
}

// StartTask prints a task starting message and returns a function to complete it
func (c *Console) StartTask(task string) func(success bool, detail string) {
	if c.quiet {
		return func(bool, string) {}
	}

	c.mu.Lock()
	fmt.Printf("[%s] %s... ", c.timestamp(), task)
	c.mu.Unlock()

	return func(success bool, detail string) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if success {
			green := c.color(colorGreen)
			reset := c.color(colorReset)
			if detail != "" {
				fmt.Printf("%s%s%s %s\n", green, checkMark, reset, detail)
			} else {
				fmt.Printf("%s%s%s\n", green, checkMark, reset)
			}
		} else {
			fmt.Printf("%s%s%s %s\n", c.color(colorYellow), crossMark, c.color(colorReset), detail)
		}
	}
}

// PrintTopics prints the MQTT topic subscriptions
func (c *Console) PrintTopics(topics []string) {
	if c.quiet {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Printf("[%s] Subscribing to topics...\n", c.timestamp())
	for i, topic := range topics {
		prefix := "├─"
		if i == len(topics)-1 {
			prefix = "└─"
		}
		fmt.Printf("           %s %s\n", prefix, topic)
	}
}

// PrintReady prints the ready message
func (c *Console) PrintReady() {
	if c.quiet {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Println()
	fmt.Printf("[%s] Ready. Waiting for data...\n", c.timestamp())
	fmt.Println()
}

// StatusLine holds the data for the periodic status line
type StatusLine struct {
	Systems     int
	CallsPerMin float64
	ActiveUnits int
	WSClients   int
}

// PrintStatus prints the status line
func (c *Console) PrintStatus(status StatusLine) {
	if c.quiet {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	gray := c.color(colorGray)
	reset := c.color(colorReset)

	fmt.Printf("%s── Status ─────────────────────────────────────────────────────%s\n", gray, reset)
	fmt.Printf("Systems: %d | Calls: %.0f/min | Units: %d active | WS clients: %d\n",
		status.Systems, status.CallsPerMin, status.ActiveUnits, status.WSClients)
}

// StatusProvider is an interface for getting status information
type StatusProvider interface {
	GetStats(ctx context.Context) (StatusLine, error)
}

// StartStatusLoop starts a goroutine that periodically prints status
func (c *Console) StartStatusLoop(ctx context.Context, provider StatusProvider, interval time.Duration) {
	if c.quiet {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, err := provider.GetStats(ctx)
				if err != nil {
					continue
				}
				c.PrintStatus(status)
			}
		}
	}()
}

// PrintShutdown prints the shutdown message
func (c *Console) PrintShutdown() {
	if c.quiet {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Println()
	fmt.Printf("[%s] Shutting down...\n", c.timestamp())
}

// PrintShutdownComplete prints the shutdown complete message
func (c *Console) PrintShutdownComplete() {
	if c.quiet {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	uptime := time.Since(c.startTime).Round(time.Second)
	fmt.Printf("[%s] Shutdown complete. Uptime: %s\n", c.timestamp(), uptime)
}

func (c *Console) timestamp() string {
	return time.Now().Format("15:04:05")
}

func (c *Console) color(code string) string {
	if c.noColor {
		return ""
	}
	// Check if stdout is a terminal
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		return ""
	}
	return code
}
