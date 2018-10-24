build:
	go get ./...
	env GOOS=linux go build -ldflags="-s -w" -o bin/warm warm/main.go

deploy:
	sls deploy
