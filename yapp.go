package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	os.Exit(main_())
}

func main_() int {

	ctx := context.Background()
	var (
		server     = flag.String("s", "localhost", "server")
		port       = flag.Int("p", 80, "tcp port")
		timeoutArg = flag.Int("t", 1000, "timeout in millisec")
	)

	flag.Parse()

	var network = fmt.Sprintf("%s:%d", *server, *port)
	var timeout = time.Duration(*timeoutArg) * time.Millisecond

	return tryPort(ctx, network, timeout)
}

func printf(ctx context.Context, format string, a ...interface{}) (n int, err error) {
	v := ctx.Value("startTime")
	startTime, ok := v.(time.Time)
	if !ok {
		return fmt.Printf(format, a...)
	}
	return fmt.Printf(startTime.Format("[2006-01-02T15:04:05]: ")+format, a...)
}

func tryPort(ctx context.Context, network string, timeout time.Duration) int {
	startTime := time.Now()
	ctx = context.WithValue(ctx, "startTime", startTime)
	conn, err := net.DialTimeout("tcp", network, timeout)
	endTime := time.Now()
	if err != nil {
		printf(ctx, "Failed. error=%v\n", err)
		return 1
	}
	defer conn.Close()
	var t = float64(endTime.Sub(startTime)) / float64(time.Millisecond)
	printf(ctx, "Connected. addr=%s time=%4.2fms\n", conn.RemoteAddr().String(), t)
	return 0
}
