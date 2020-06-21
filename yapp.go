package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func print(ctx context.Context, msg string) error {
	logChan, ok := ctx.Value("logChan").(chan string)
	if !ok {
		fmt.Println("logger not found")
		return fmt.Errorf("logger not found")
	}
	//fmt.Printf("send msg to logger chan(%v): %v\n", logChan, msg)
	logChan <- msg
	//fmt.Printf("send msg to logger: %v --> return\n", msg)
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

type server struct {
	host    string
	port    int
	comment string
}

func main_() int {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		/*
			server     = flag.String("s", "localhost", "server")
			port       = flag.Int("p", 80, "tcp port")
		*/
		timeoutArg = flag.Int("t", 500, "timeout in millisec")
		list       = flag.String("f", "", "server list csv file formatted in \"ServerName\\tPortNumber\\tComment\"")
		conc       = flag.Int("c", 5, "concurrency")
		serverList = []server{}
	)

	flag.Parse()

	var timeout = time.Duration(*timeoutArg) * time.Millisecond
	ctx = context.WithValue(ctx, "timeout", timeout)

	// read server list
	if *list != "" {
		file, err := os.Open(*list)
		if err != nil {
			fmt.Println("Cannot open file: " + *list)
			return 1
		}
		defer file.Close()

		reader := csv.NewReader(file)
		var line []string

		for {
			line, err = reader.Read()
			if err != nil {
				break
			}
			if len(line) != 3 {
				fmt.Printf("csv format error: %v\n", line)
				return 1
			}
			host := line[0]
			port_str := line[1]
			comment := line[2]
			port, err := strconv.Atoi(port_str)
			if err != nil {
				fmt.Printf("csv format error: %v\n", line)
				fmt.Println("port number error: " + port_str)
				return 1
			}
			serverList = append(serverList, server{
				host:    host,
				port:    port,
				comment: comment,
			})

		}
		//fmt.Printf("serverList=%v\n", serverList)
	}

	wg := new(sync.WaitGroup)

	// start logger goroutine
	logChan := make(chan string)
	//logChan := make(chan string, *conc)
	ctx = context.WithValue(ctx, "logChan", logChan)
	wg.Add(1)
	go func(logch chan string) {
		for {

			//fmt.Printf("logger: select logch:%v\n", logch)
			select {
			case outs := <-logch:
				//fmt.Println("logger: received :" + outs)
				sc := bufio.NewScanner(strings.NewReader(outs))
				for sc.Scan() {
					fmt.Println(time.Now().Format("[2006-01-02T15:04:05] ") + sc.Text())
				}
			case <-ctx.Done():
				wg.Done()
				return
			default:
			}
		}
	}(logChan)

	// start ping goroutines
	wg2 := new(sync.WaitGroup)
	if len(serverList) < *conc {
		*conc = len(serverList)
	}
	reqChan := make(chan server)
	for i := 0; i < *conc; i++ {
		wg2.Add(1)
		go func() {
			for svr := range reqChan {
				err := tryPort(ctx, svr.host, svr.port)
				if err != nil {
					print(ctx, fmt.Sprintf("Failed. error=%v", err))
					err = ping(ctx, svr.host)
					if err != nil {
						print(ctx, fmt.Sprintf("%v", err))
					}
					err = traceroute(ctx, svr.host)
					if err != nil {
						print(ctx, fmt.Sprintf("%v", err))
					}
				}
			}
			wg2.Done()
		}()
	}

	// send request to ping goroutines
	for _, svr := range serverList {
		reqChan <- svr
	}

	// wait for all ping goroutines finished
	close(reqChan)
	wg2.Wait()

	// wait for logging goroutine finished
	cancel()
	wg.Wait()

	return 0
}

func main() {
	os.Exit(main_())
}
