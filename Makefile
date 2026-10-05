IMG ?= pv-guard:latest

.PHONY: test build docker-build deploy undeploy

test:
	go vet ./...
	go test ./...

build:
	go build -o bin/pv-guard ./cmd

docker-build:
	docker build -t $(IMG) .

deploy:
	kubectl kustomize config | sed 's#image: pv-guard:latest#image: $(IMG)#' | kubectl apply -f -

undeploy:
	kubectl delete -k config --ignore-not-found
