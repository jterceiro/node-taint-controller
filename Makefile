.DEFAULT_GOAL := help

# Image URL to use for all building/pushing image targets
IMG ?= ghcr.io/jterceiro/node-taint-controller:latest

# Kubernetes version used by envtest
ENVTEST_K8S_VERSION ?= 1.31.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# LOCALBIN points to a local directory where tools are installed.
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool binaries
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST         ?= $(LOCALBIN)/setup-envtest
KUSTOMIZE       ?= $(LOCALBIN)/kustomize
GOLANGCI_LINT   ?= $(LOCALBIN)/golangci-lint

## Tool versions
CONTROLLER_GEN_VERSION ?= v0.16.5
ENVTEST_VERSION         ?= release-0.19
KUSTOMIZE_VERSION       ?= v5.4.3
GOLANGCI_LINT_VERSION   ?= v1.64.8

# Setting SHELL to bash allows bash commands to be used in recipes, e.g. 'set -o pipefail'.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

.PHONY: help
help: ## Display this help (default target)
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
	$(CONTROLLER_GEN) rbac:roleName=node-taint-controller-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: test
test: envtest ## Run tests
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint (set FIX=1 to also apply fixes)
	$(GOLANGCI_LINT) run $(if $(FIX),--fix,) ./...

##@ Build

.PHONY: build
build: ## Build manager binary
	go build -o bin/manager ./cmd/

.PHONY: docker-build
docker-build: ## Build docker image with the manager (IMG=<registry/image:tag>)
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push docker image with the manager (IMG=<registry/image:tag>)
	docker push $(IMG)

##@ Deployment

.PHONY: deploy
deploy: kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config
	(cd config && $(KUSTOMIZE) edit set image controller=$(IMG))
	$(KUSTOMIZE) build config | kubectl apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config
	$(KUSTOMIZE) build config | kubectl delete --ignore-not-found -f -

##@ Build Dependencies

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_GEN_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name.
# Usage: $(call go-install-tool,<tool-bin-path>,<package>,<version>)
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)@$(3)" ;\
GOBIN=$(LOCALBIN) go install $(2)@$(3) ;\
rm -rf $$TMP_DIR ;\
}
endef
