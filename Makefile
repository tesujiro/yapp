build:
	go build

all:
	go build
	GOOS=linux GOARCH=amd64 go build -o linux-amd64/yapp
	GOOS=windows GOARCH=amd64 go build -o windows-amd64/yapp
