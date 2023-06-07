# Exports for go.mk
export APP_NAME 			:= resource-advisor

# Download wanted go.mk version automatically if not present.
BASE_VERSION  := 0436a53
BASE_MAKE     := go-${BASE_VERSION}.mk
ifeq ($(wildcard ${BASE_MAKE}),)
$(shell gh api -H 'Accept: application/vnd.github.v3.raw' 'repos/elisasre/baseconfig/contents/go.mk?ref=${BASE_VERSION}' > ${BASE_MAKE})
endif

include ${BASE_MAKE}

.PHONY: clean run build-subset rename-files checksum validate-go-mk

clean:
	git clean -Xdf

run: run/${SYS_GOOS}/${SYS_GOARCH}
run/%: go-build/%
	$(BUILD_OUTPUT)

SUBSET := linux/amd64 windows/amd64 darwin/amd64 darwin/arm64
build-subset: ${SUBSET:%=go-build/%} ${SUBSET:%=checksum/%} ${SUBSET:%=rename-files/%}

checksum/%:
	sha256sum -b target/bin/${*}/resource-advisor > target/bin/${*}/resource-advisor.txt

SLASH 	:= /
DASH 	:= -
rename-files/%:
	${eval filename = resource-advisor-$(subst $(SLASH),$(DASH),$(*))}
	mv target/bin/${*}/resource-advisor target/bin/${*}/$(filename);
	mv target/bin/${*}/resource-advisor.txt target/bin/${*}/$(filename).txt;

validate-go-mk: ## Check that go.mk hasn't been manually modified
	$(info Fetching go.mk: ${FETCH_BASE_MAKE})
	git diff --exit-code -- ${BASE_MAKE}

