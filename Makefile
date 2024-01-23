.Phony: run-client run-server

run-client:
		cd client && go run main.go

run-server:
		cd server && go run main.go hub.go clients.go