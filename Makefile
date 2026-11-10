go_environment = GOTOOLCHAIN=local

.PHONY: build check proto

build:
	@$(go_environment) go build -trimpath -o build/traderswiftd .
	@$(go_environment) go build -trimpath -o build/hub_admin_key_b ./cmd/hub_admin_key_b
	@echo "build: build/traderswiftd build/hub_admin_key_b"

check: build
	@test -z "$$($(go_environment) gofmt -l .)" || { echo "gofmt needed:"; $(go_environment) gofmt -l .; exit 1; }
	@$(go_environment) go vet ./...
	@$(go_environment) go test -count=1 -v ./network/ > build/scenarios.log 2>&1 || { cat build/scenarios.log; exit 1; }
	@grep -E '^(network|first contact) scenarios' build/scenarios.log
	@! grep -q lockbox go.mod || { echo "go.mod names lockbox"; exit 1; }
	@echo "check: PASS"

proto:
	./generate_proto.sh
