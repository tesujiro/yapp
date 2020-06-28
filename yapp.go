package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
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
	logChan <- msg
	return nil
}

func execute(ctx context.Context, command string) error {
	args := strings.Split(command, " ")
	cmd := exec.Command(args[0], args[1:]...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	cmdline := fmt.Sprintf("\n$ %v\n", command)
	err := cmd.Run()
	if err != nil {
		print(ctx, cmdline+fmt.Sprint(err))
	}
	print(ctx, cmdline+stdout.String())
	return err
}

func showConfig(ctx context.Context) error {

	var command string
	switch runtime.GOOS {
	case "windows":
		command = fmt.Sprintf("ipconfig")
	default:
		command = fmt.Sprintf("ifconfig -a")
	}
	err := execute(ctx, command)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "windows":
		command = fmt.Sprintf("route PRINT")
	case "darwin":
		command = fmt.Sprintf("netstat -rn")
	default:
		command = fmt.Sprintf("route")
	}
	return execute(ctx, command)

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
	var t = float64(endTime.Sub(startTime)) / float64(time.Millisecond)
	if err != nil {
		print(ctx, fmt.Sprintf("Connection failed.\tserver=%v port=%v time=%4.2fms error=%v", server, port, t, err))
		return err
	}
	defer conn.Close()
	print(ctx, fmt.Sprintf("Connection succeeded.\tserver=%v port=%v time=%4.2fms", server, port, t))
	return nil
}

type server struct {
	host    string
	port    int
	comment string
}

func readCsv(filepath string) ([]server, error) {
	if filepath == "" {
		return nil, nil
	}

	file, err := os.Open(filepath)
	if err != nil {
		return nil, errors.New("Cannot open file: " + filepath)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	var line []string

	serverList := []server{}
	for {
		line, err = reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv read error: %v\n", err)
		}
		if len(line) != 3 {
			return nil, fmt.Errorf("csv format error: %v\n", line)
		}
		host := line[0]
		port_str := line[1]
		comment := line[2]
		port, err := strconv.Atoi(port_str)
		if err != nil {
			fmt.Println("port number error: " + port_str)
			return nil, fmt.Errorf("csv format error: %v\n", line)
		}
		serverList = append(serverList, server{
			host:    host,
			port:    port,
			comment: comment,
		})
	}
	return serverList, nil
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
	)

	flag.Parse()

	var timeout = time.Duration(*timeoutArg) * time.Millisecond
	ctx = context.WithValue(ctx, "timeout", timeout)

	// read server list
	serverList, err := readCsv(*list)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return 1
	}

	if len(serverList) < *conc {
		*conc = len(serverList)
	}

	wg := new(sync.WaitGroup)

	// start logger goroutine
	logChan := make(chan string)
	//logChan := make(chan string, *conc)
	ctx = context.WithValue(ctx, "logChan", logChan)
	wg.Add(1)
	go func(logch chan string) {
		hostname, _ := os.Hostname()
		for {

			//fmt.Printf("logger: select logch:%v\n", logch)
			select {
			case outs := <-logch:
				//fmt.Println("logger: received :" + outs)
				sc := bufio.NewScanner(strings.NewReader(outs))
				for sc.Scan() {
					fmt.Println(time.Now().Format("[2006/01/02 15:04:05 ") + hostname + "] " + sc.Text())
				}
			case <-ctx.Done():
				wg.Done()
				return
			default:
			}
		}
	}(logChan)

	// show config
	if err := showConfig(ctx); err != nil {
		fmt.Println(err)
		return 1
	}

	// start cache check goroutine
	checkServerChan := make(chan server)
	alreadyCheckedChan := make(chan bool)
	go func() {
		checkedServer := make(map[string]bool)
		var yet bool
		for s := range checkServerChan {
			if checkedServer[s.host] {
				yet = true
			} else {
				yet = false
				checkedServer[s.host] = true
			}
			alreadyCheckedChan <- yet
		}
	}()

	// start port ping goroutines
	wg2 := new(sync.WaitGroup)
	reqChan := make(chan server)
	for i := 0; i < *conc; i++ {
		wg2.Add(1)
		go func() {
			for svr := range reqChan {
				err := tryPort(ctx, svr.host, svr.port)
				if err != nil && err.(*net.OpError).Timeout() {
					checkServerChan <- svr
					if !<-alreadyCheckedChan {
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
