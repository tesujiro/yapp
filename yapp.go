package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
		timeoutArg = flag.Int("t", 500, "timeout in millisec")
	)

	flag.Parse()

	var timeout = time.Duration(*timeoutArg) * time.Millisecond

	return tryPort(ctx, *server, *port, timeout)
}

func printf(ctx context.Context, format string, a ...interface{}) (n int, err error) {
	v := ctx.Value("startTime")
	startTime, ok := v.(time.Time)
	if !ok {
		return fmt.Printf(format, a...)
	}
	return fmt.Printf(startTime.Format("[2006-01-02T15:04:05]: ")+format, a...)
}

func execute(command string) error {
	args := strings.Split(command, " ")
	cmd := exec.Command(args[0], args[1:]...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	fmt.Println("$ ")
	fmt.Println("$ " + command)
	err := cmd.Run()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(stdout.String())
	return err
}

func ping(server string) error {

	const count = 5

	var command string
	switch runtime.GOOS {
	case "windows":
		command = fmt.Sprintf("ping -n %v %s", count, server)
	default:
		command = fmt.Sprintf("ping -c %v -i 0.1 %s", count, server)
	}

	return execute(command)
}

func traceroute(server string) error {

	var command string
	switch runtime.GOOS {
	case "windows":
		command = fmt.Sprintf("tracert %s", server)
	default:
		command = fmt.Sprintf("traceroute -I %s", server)
	}

	return execute(command)
}

func tryPort(ctx context.Context, server string, port int, timeout time.Duration) int {
	startTime := time.Now()
	ctx = context.WithValue(ctx, "startTime", startTime)
	network := fmt.Sprintf("%s:%d", server, port)
	conn, err := net.DialTimeout("tcp", network, timeout)
	endTime := time.Now()
	if err != nil {
		printf(ctx, "Failed. error=%v\n", err)
		ping(server)
		traceroute(server)
		return 1
	}
	defer conn.Close()
	var t = float64(endTime.Sub(startTime)) / float64(time.Millisecond)
	printf(ctx, "Connected. addr=%s time=%4.2fms\n", conn.RemoteAddr().String(), t)
	return 0
}
