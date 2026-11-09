# cgo-gui — copie de vérification (runbook gui-repo § Vérification)
stub-vet:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...

stub-build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...

win-build:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...

linux-build:
	CGO_ENABLED=1 go build ./...

test:
	go test ./...

verify: stub-vet stub-build win-build test
