DEMO_COMPOSE := demo/compose.yaml
DEMO_ADMIN_TOKEN ?= codestation-demo-admin

.PHONY: build bins package provider-build devkit-image devkit-release test provider-test registry-test devkit-test extension-test verify release-check demo-keys demo-build demo-seed demo-up demo-check demo-status demo-logs demo-down demo-clean demo-reset

bins:
	./scripts/download-bins.sh

build:
	cd provider && go build ./...
	cd registry && go build ./...
	npm run build

package: bins provider-build
	npm run package

provider-build:
	./scripts/build-provider.sh

test: provider-test registry-test devkit-test extension-test

provider-test:
	cd provider && go test ./...

registry-test:
	cd registry && go test ./...

devkit-test:
	cd devkit && go test ./...

devkit-image: bins provider-build
	docker build -f devkit/Dockerfile -t codestation-devkit:local .

devkit-release: bins provider-build
	./scripts/build-devkit.sh

extension-test:
	npm test

verify: bins
	./scripts/verify-package.sh examples/packages/minimal

release-check: bins
	./scripts/release-check.sh

demo-keys:
	./scripts/generate-demo-keys.sh

demo-build: bins provider-build devkit-image demo-keys
	CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" docker compose -f $(DEMO_COMPOSE) build

demo-seed: devkit-image demo-keys
	CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" docker compose -f $(DEMO_COMPOSE) up --build -d registry
	CODESTATION_REGISTRY_URL="http://127.0.0.1:$${REGISTRY_PORT:-8081}" \
		CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" \
		./scripts/seed-demo.sh

demo-up: bins demo-keys demo-seed
	CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" docker compose -f $(DEMO_COMPOSE) up --build -d
	$(MAKE) demo-check

demo-check:
	CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" ./scripts/check-demo.sh

demo-status:
	CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" docker compose -f $(DEMO_COMPOSE) ps

demo-logs:
	CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" docker compose -f $(DEMO_COMPOSE) logs -f

demo-down:
	CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" docker compose -f $(DEMO_COMPOSE) down

demo-clean:
	rm -f demo/keys/demo-signing-private.pem demo/keys/demo-signing-public.pem demo/keys/demo-ca.crt demo/keys/demo-tls.key demo/keys/demo-tls.crt keys/demo-2026.pem

demo-reset:
	CODESTATION_ADMIN_TOKEN="$(DEMO_ADMIN_TOKEN)" docker compose -f $(DEMO_COMPOSE) down -v
	$(MAKE) demo-clean
