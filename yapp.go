package main

import (
	"bufio"
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

func print(ctx context.Context, msg string) error {
	logChan, ok := ctx.Value("logChan").(chan string)
	if !ok {
		fmt.Println("logger not found")
		return fmt.Errorf("logger not found")
	}
	logDone, ok := ctx.Value("logDone").(chan struct{})
	if !ok {
		fmt.Println("logger done channel not found")
		return fmt.Errorf("logger done channel not found")
	}
	logChan <- msg
	<-logDone
	return nil
}

func execute(ctx context.Context, command string) error {
	args := strings.Split(command, " ")
	cmd := exec.Command(args[0], args[1:]...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	print(ctx, "")
	print(ctx, fmt.Sprint("$ "+command))
	err := cmd.Run()
	if err != nil {
		print(ctx, fmt.Sprint(err))
	}
	print(ctx, stdout.String())
	return err
}

func ping(ctx context.Context, server string) error {

	const count = 5

	var command string
	switch runtime.GOOS {
	case "windows":
		command = fmt.Sprintf("ping -n %v -w 100 %s", count, server)
	default:
		command = fmt.Sprintf("ping -c %v -i 0.1 %s", count, server)
	}

	return execute(ctx, command)
}

func traceroute(ctx context.Context, server string) error {

	var command string
	switch runtime.GOOS {
	case "windows":
		command = fmt.Sprintf("tracert -w 100 -h 15 -d %s", server)
	default:
		command = fmt.Sprintf("traceroute -w 1 -m 15 -I %s", server)
	}

	return execute(ctx, command)
}

func tryPort(ctx context.Context, server string, port int) error {
	startTime := time.Now()
	//ctx = context.WithValue(ctx, "startTime", startTime)
	network := fmt.Sprintf("%s:%d", server, port)
	timeout, ok := ctx.Value("timeout").(time.Duration)
	if !ok {
		return fmt.Errorf("timeout not found")
	}
	conn, err := net.DialTimeout("tcp", network, timeout)
	endTime := time.Now()
	if err != nil {
		return err
	}
	defer conn.Close()
	var t = float64(endTime.Sub(startTime)) / float64(time.Millisecond)
	print(ctx, fmt.Sprintf("Connected. addr=%s time=%4.2fms", conn.RemoteAddr().String(), t))
	return nil
}

func main_() int {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		server     = flag.String("s", "localhost", "server")
		port       = flag.Int("p", 80, "tcp port")
		timeoutArg = flag.Int("t", 500, "timeout in millisec")
		//list       = flag.String("f", "", "server list formatted in \"ServerName\\tPortNumber\\tComment\"")
	)

	flag.Parse()

	var timeout = time.Duration(*timeoutArg) * time.Millisecond
	ctx = context.WithValue(ctx, "timeout", timeout)

	// start logger goroutine
	logChan := make(chan string)
	ctx = context.WithValue(ctx, "logChan", logChan)
	logDone := make(chan struct{})
	ctx = context.WithValue(ctx, "logDone", logDone)
	go func() {
		for {
			select {
			case outs := <-logChan:
				sc := bufio.NewScanner(strings.NewReader(outs))
				for sc.Scan() {
					fmt.Println(time.Now().Format("[2006-01-02T15:04:05] ") + sc.Text())
				}
				logDone <- struct{}{}
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	err := tryPort(ctx, *server, *port)
	if err != nil {
		print(ctx, fmt.Sprintf("Failed. error=%v", err))
		err = ping(ctx, *server)
		if err != nil {
			print(ctx, fmt.Sprintf("%v", err))
		}
		err = traceroute(ctx, *server)
		if err != nil {
			print(ctx, fmt.Sprintf("%v", err))
		}
		return 1
	}
	return 0
}

func main() {
	os.Exit(main_())
}
