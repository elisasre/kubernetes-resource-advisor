BINARY_NAME := resource-advisor
ifeq ($(USE_JSON_OUTPUT), 1)
GOTEST_REPORT_FORMAT := -json
endif

.PHONY: clean deps test gofmt run ensure build

clean:
	git clean -Xdf

deps:
	GO111MODULE=off go get -u golang.org/x/lint/golint

test:
	GO111MODULE=on go test ./... -v -coverprofile=gotest-coverage.out $(GOTEST_REPORT_FORMAT) > gotest-report.out && cat gotest-report.out || (cat gotest-report.out; exit 1)
	GO111MODULE=off golint -set_exit_status cmd/... pkg/... > golint-report.out && cat golint-report.out || (cat golint-report.out; exit 1)
	GO111MODULE=on go vet -mod vendor ./...
	./hack/gofmt.sh
	git diff --exit-code go.mod go.sum

gofmt:
	./hack/gofmt.sh

ensure:
	GO111MODULE=on go mod tidy
	GO111MODULE=on go mod vendor

run: build
	./bin/$(BINARY_NAME)

build:
	rm -f bin/$(BINARY_NAME)
	GO111MODULE=on go build -v -o bin/$(BINARY_NAME) ./cmd
