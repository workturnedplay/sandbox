package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	listenIP   = "127.4.5.19"
	listenPort = "4431"
	targetHost = "5ea1.playfabapi.test"
	bindAddr   = listenIP + ":" + listenPort
	dialAddr   = targetHost + ":" + listenPort
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. Warm up Portmaster process-table index
	prewarmPortmaster(ctx)

	// 2. Start target listener
	ready := make(chan struct{})
	go startListener(ctx, ready)
	<-ready

	// 3. Configure dialer
	dialer := net.Dialer{
		Timeout: 200 * time.Millisecond, // Fast timeout for quick ETW catch-up
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 1 * time.Second}
				return d.DialContext(ctx, "udp4", "127.0.0.1:53")
			},
		},
	}

	// 4. Retry loop with backoff for PID race recovery
	var conn net.Conn
	var err error

	for i := 0; i < 300; i++ {
		start := time.Now()

		// Force Portmaster to process the DNS query and bind the IP-to-Domain mapping to our PID
		if _, err = dialer.Resolver.LookupHost(ctx, targetHost); err != nil {
			fmt.Fprintf(os.Stderr, "❌ DNS lookup failed: %v\n", err)
			os.Exit(1)
		}

		// Give Portmaster's ETW daemon 250ms to write the PID -> Domain -> IP table to the WFP driver
		time.Sleep(250 * time.Millisecond)

		// Assign to outer 'conn' variable (avoiding := shadowing)
		conn, err = dialer.DialContext(ctx, "tcp4", dialAddr)
		if err == nil {
			fmt.Printf("✅ Connected on attempt %d (%v)\n", i+1, time.Since(start).Round(time.Millisecond))
			break
		}
		fmt.Printf("⚠️ Attempt %d dropped (%v), retrying...\n", i+1, err)
		time.Sleep(100 * time.Millisecond)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection persistently blocked: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	payload := fmt.Sprintf("PING %s HTTP/1.1\r\n\r\n", targetHost)
	_, _ = conn.Write([]byte(payload))
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	fmt.Printf("Response: %q\n", string(buf[:n]))
}

func prewarmPortmaster(ctx context.Context) {
	lc := net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return
	}
	defer l.Close()

	go func() {
		c, err := l.Accept()
		if err == nil {
			c.Close()
		}
	}()

	var d net.Dialer
	d.Timeout = 200 * time.Millisecond
	if c, err := d.DialContext(ctx, "tcp4", l.Addr().String()); err == nil {
		c.Close()
	}
	time.Sleep(100 * time.Millisecond)
}

func startListener(ctx context.Context, ready chan<- struct{}) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				err = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
			return err
		},
	}

	listener, err := lc.Listen(ctx, "tcp4", bindAddr)
	if err != nil {
		fmt.Printf("Listen err: %v\n", err)
		return
	}
	close(ready)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 256)
			n, _ := c.Read(buf)
			_, _ = c.Write([]byte("PONG " + string(buf[:n])))
		}(conn)
	}
}