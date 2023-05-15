.DEFAULT_GOAL := help

COMMA := ,
SPACE := $(subst ,, )

GO_VERSION	?= 1.20.4
TOOLS_DIR	:= .tools
GO 			:= ${TOOLS_DIR}/go/go${GO_VERSION}

# If wanted version of Go isn't installed yet we use sane defaults.
# This will allow us to trigger Go installation without error messages when these variables are evaluated.
ifneq ($(wildcard ${GO}),)
GO_BUILD_MATRIX		:= $(shell ${GO} tool dist list)
SYS_GOOS			:= $(shell ${GO} env GOOS)
SYS_GOARCH			:= $(shell ${GO} env GOARCH)
else
GO_BUILD_MATRIX		:= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
ifeq ($(shell uname -s),Darwin)
SYS_GOOS			:= darwin
else
SYS_GOOS			:= linux
endif
ifneq ($(filter arm%,$(shell uname -p)),)
SYS_GOARCH			:= arm64
else
SYS_GOARCH			:= amd64
endif
endif

GO_BUILD_TARGETS	:= ${GO_BUILD_MATRIX:%=go-build/%}
GOARCH				= $(notdir ${*})
GOOS				= ${*:%/${GOARCH}=%}
BUILD_OUTPUT		= target/bin/${GOOS}/${GOARCH}/${APP_NAME}
CONTAINER_PLATFORMS	:= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

# Tools that can be installed with `go install` command.
GOLANGCI_LINT_V		?= v1.52.2
SWAG_V				?= v1.8.12
GO_LICENSES_V		?= v1.6.0
GOVULNCHECK_V		?= latest
COVMERGE_V			?= master
GOGET_GOLANGCI_LINT	:= ${TOOLS_DIR}/github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_V}
GOGET_SWAG			:= ${TOOLS_DIR}/github.com/swaggo/swag/cmd/swag@${SWAG_V}
GOGET_GO_LICENSES	:= ${TOOLS_DIR}/github.com/google/go-licenses@${GO_LICENSES_V}
GOGET_GOVULNCHECK	:= ${TOOLS_DIR}/golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_V}
GOGET_COVMERGE      := ${TOOLS_DIR}/github.com/wadey/gocovmerge@${COVMERGE_V}
GOGET_TOOLS			:= $(foreach v, $(filter GOGET_%,${.VARIABLES}), ${${v}})

# Lazily evaluated helpers for `go install`.
MAJOR_VER	= $(firstword $(subst ., ,$(lastword $(subst @, ,${@}))))
LAST_PART	= $(notdir $(firstword $(subst @, ,${@})))
BIN_PATH	= ${PWD}/${@D}
BIN_NAME	= $(if $(filter ${LAST_PART},$(MAJOR_VER)),$(notdir ${BIN_PATH}),${LAST_PART})

## Variables that can be overwritten by parent makefile via env.

# Common variables
APP_NAME			?=
LAST_COMMIT_SHA		?= $(shell git rev-parse HEAD)

# Go stuff
CGO_ENABLED			?= 0
GO_BUILD_TARGET		?= ./cmd/${APP_NAME}
GO_BUILD_FLAGS		?=
GO_REPORTS_DIR		?= target/reports
GO_UNIT_COV_FILE	?= ${GO_REPORTS_DIR}/unit-test-coverage.out
GO_INT_COV_FILE		?= ${GO_REPORTS_DIR}/integration-test-coverage.out
GO_TOTAL_COV_FILE	?= ${GO_REPORTS_DIR}/total-test-coverage.out
GO_INT_TEST_PKG		?= ${GO_BUILD_TARGET}
GO_UNIT_TEST_FLAGS	?= -race -covermode atomic -coverprofile=${GO_UNIT_COV_FILE} ./...
GO_INT_TEST_FLAGS	?= -race -covermode atomic -tags=integration -coverpkg=$(subst ${SPACE},${COMMA},$(shell ${GO} list ./...)) -coverprofile=${GO_INT_COV_FILE} ${GO_INT_TEST_PKG}
GOLANGCI_LINT_FLAGS	?= --fix
SWAG_FLAGS			?= --parseVendor --parseInternal --parseDependency --generalInfo api/api.go ./api/
LICENSES_CSV		?= ${GO_REPORTS_DIR}/licenses.csv

# Docker stuff
DOCKER_IMAGE_NAME	?= quay.io/elisaoyj/${APP_NAME}
DOCKER_IMAGE_TAGS	?= sha-${LAST_COMMIT_SHA}
DOCKER_EXTRA_CTX	?= --build-context bin=$(dir ${BUILD_OUTPUT})
DOCKERFILE		?= Dockerfile
DOCKER_BUILD_ARGS	?= -f ${DOCKERFILE} --progress plain --load
DOCKER_BUILD_CTX	?= .

# Image labels
OCI_title			?= ${APP_NAME}
OCI_description		?=
OCI_url				?= $(shell git remote get-url origin)
OCI_source			?= $(shell git remote get-url origin)
OCI_version			?= sha-${LAST_COMMIT_SHA}
OCI_created			?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
OCI_revision		?= ${LAST_COMMIT_SHA}
OCI_licenses		?=
OCI_authors			?= SoSe/SRE
OCI_vendor			?= Elisa
ALL_OCI_LABELS		= $(foreach v, $(filter OCI_%,${.VARIABLES}), ${v:OCI_%=%}="${${v}}")
LABEL_PREFIX		:= --label org.opencontainers.image.

.PHONY: go go-tools go-version go-lint go-vuln-check go-license-report go-swagger go-ensure go-unit-test go-integration-test go-merge-coverages go-build go-build-all image-build image-build-all

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nExample:\n  \033[36mmake go-build/linux/amd64\033[0m\n  Build linux binary for amd64 architecture\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

go: ${GO} ## Install wanted Go version
${GO}:
	GOBIN=${PWD}/$(dir ${GO}) go install -mod=readonly golang.org/dl/go${GO_VERSION}@latest
	${GO} download

go-tools: ${GOGET_TOOLS} ## Install all Go tools
${GOGET_TOOLS}: ${GO}
	@mkdir -p ${BIN_PATH}
	cd $(shell mktemp -d) && GOFLAGS='' GOBIN='${BIN_PATH}' ${PWD}/${GO} install ${@:${TOOLS_DIR}/%=%}
	@mv ${BIN_PATH}/${BIN_NAME} ${@}

go-version: ## Print wanted Go version
	@echo ${GO_VERSION}

go-lint: ${GOGET_GOLANGCI_LINT} ## Run golangci-linter
	${GOGET_GOLANGCI_LINT} run ${GOLANGCI_LINT_FLAGS}

go-vuln-check: ${GOGET_GOVULNCHECK} ## Run Go vulnerability scanner
	${GOGET_GOVULNCHECK} ./...

# There's a bug in go-licenses: https://github.com/google/go-licenses/issues/193
go-license-report: ${GOGET_GO_LICENSES} ## Generate Go license report
	$(if $(dir ${LICENSES_CSV}),$(shell mkdir -p $(dir ${LICENSES_CSV})))
	GOROOT=$(shell ${GO} env GOROOT) ${GOGET_GO_LICENSES} report ${GO_BUILD_TARGET} > ${LICENSES_CSV} 2> /dev/null

go-swagger: ${GOGET_SWAG} ## Run swagger generator
	${GOGET_SWAG} init ${SWAG_FLAGS}

go-ensure: ${GO} go-tidy ## Tidy and vendor Go dependencies
	${GO} mod vendor

go-tidy: ${GO} ## Tidy Go dependencies
	${GO} mod tidy

go-verify-mod-files: go-tidy ## Verify that tidy didn't modify deps
	git diff --exit-code -- go.mod go.sum

go-unit-test: ${GO} ## Run Go unit tests
	$(if $(dir ${GO_UNIT_COV_FILE}),$(shell mkdir -p $(dir ${GO_UNIT_COV_FILE})))
	${GO} test ${GO_UNIT_TEST_FLAGS}

go-integration-test: ${GO} ## Run Go integration tests
	$(if $(dir ${GO_INT_COV_FILE}),$(shell mkdir -p $(dir ${GO_INT_COV_FILE})))
	${GO} test ${GO_INT_TEST_FLAGS}

go-merge-coverages: ${GOGET_COVMERGE} ## Merge unit and integration profiles
	${GOGET_COVMERGE} ${GO_UNIT_COV_FILE} ${GO_INT_COV_FILE} > ${GO_TOTAL_COV_FILE}

go-build: go-build/${SYS_GOOS}/${SYS_GOARCH} ## Build Go binary for current system
go-build-all: ${GO_BUILD_TARGETS} ## Build Go binary for all supported platforms
go-build/%: ${GO}
	$(if ${APP_NAME},,$(error APP_NAME not set))
	GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=${CGO_ENABLED} \
		${GO} build ${GO_BUILD_FLAGS} -o ${BUILD_OUTPUT} ${GO_BUILD_TARGET}

image-build: image-build/${SYS_GOOS}/${SYS_GOARCH} ## Build docker image for current system
image-build-all: ${CONTAINER_PLATFORMS:%=image-build/%} ## Build docker image for all supported platforms
image-build/%:
	docker buildx build --platform ${*} \
		${DOCKER_IMAGE_TAGS:%=--tag ${DOCKER_IMAGE_NAME}:%} \
		${ALL_OCI_LABELS:%=${LABEL_PREFIX}%} \
		${DOCKER_BUILD_ARGS} ${DOCKER_EXTRA_CTX} ${DOCKER_BUILD_CTX}

.image-push: ## Push docker image with all tags
	docker push --all-tags ${DOCKER_IMAGE_NAME}
