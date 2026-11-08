PROJECT:=go-admin

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o $(PROJECT) .

# make build-linux
build-linux:
	@docker build -t go-admin:latest .
	@echo "build successful"

build-sqlite:
	go build -tags sqlite3 -ldflags="-w -s" -o $(PROJECT) .

# make run
run:
    # delete go-admin-api container
    #
    # stop then rm, rather than `rm -f`. The force flag kills a running
    # container with SIGKILL and no grace at all, so restarting locally cut
    # short every shutdown this application does - the drain window was never
    # once reached on a developer's machine. --timeout has to cover
    # extend.shutdown's drain + server + cleanup; checksilent's
    # docker-stop-cuts-shutdown-short check compares it against
    # config/settings.yml. On a container that has already stopped, stop is a
    # no-op and the removal is unchanged.
	@if [ $(shell docker ps -aq --filter name=go-admin --filter publish=8000) ]; then docker stop --timeout 30 go-admin && docker rm go-admin; fi

    # 启动方法一 run go-admin-api container  docker-compose 启动方式
    # 进入到项目根目录 执行 make run 命令
	@docker-compose up -d

	@echo "go-admin service is running..."

	# delete Tag=<none> 的镜像
	@docker image prune -f
	@docker ps -a | grep "go-admin"

stop:
    # delete go-admin-api container
	@if [ $(shell docker ps -aq --filter name=go-admin --filter publish=8000) ]; then docker-compose down; fi


# -race is worth the extra minute here: common/actions reuses model instances
# across concurrent requests, so a Generate() that returns in place instead of
# a copy leaks data between them - and that is invisible to a single-threaded
# test run.
.PHONY: test
test:
	go test -race -cover ./...

# The end-to-end install, which `test` above cannot reach: test/e2e-apporder
# is its own module, so `./...` in this one does not include it. It builds a
# go-admin binary with the example application linked in and drives
# `migrate install` / `migrate uninstall` against a real database.
#
# Its own target rather than a line in the CI workflow, so the one thing in
# the build that exercises installing an application is also the one thing
# somebody can run before pushing.
.PHONY: test-e2e
test-e2e:
	cd test/e2e-apporder && go test ./... -count=1

# Reports the failures that do not announce themselves - see
# tools/checksilent. Exits non-zero on an ERROR; the one WARN-level check
# prints and does not fail the build.
#
# Pass UI_DIR to enable the cross-repository menu-name check, which is skipped
# without it:  make checksilent UI_DIR=../go-admin-ui/src
.PHONY: checksilent
checksilent:
ifdef UI_DIR
	go run ./tools/checksilent -ui-dir $(UI_DIR)
else
	go run ./tools/checksilent
endif

# gofmt as a gate, not a rewrite. CI cannot commit, and a target that quietly
# reformats hides what it touched, so this reports and fails instead. `gofmt -l`
# prints the files it would rewrite and nothing at all when there are none, so
# that list is both the failure message and the instructions for fixing it.
#
# The tree reached zero unformatted files once; without something holding it
# there it drifts back, which is how the previous batch grew to 26 files -
# mostly a missing newline at the end of the file, which no reviewer notices.
.PHONY: fmt-check
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt would rewrite these files. Run 'gofmt -w .' and commit the result:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# make deploy
deploy:

	make build-linux
	make run
