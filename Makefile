.PHONY: build execute setup setup-redis setup-etcd clean

ETCD_VER	:=	v3.6.7

setup: setup-redis setup-etcd

setup-redis:
	@if [ "$$(docker ps -aq -f name=distributedqueue)" ]; then \
		echo "Redis container exists, starting it..."; \
		docker start distributedqueue 2>/dev/null || true; \
	else \
		echo "Creating Redis container..."; \
		docker run -d --name distributedqueue -p 6543:6379 redis:latest; \
	fi

setup-etcd:
	@if [ "$$(docker ps -aq -f name=etcd-gcr-${ETCD_VER})" ]; then \
		echo "etcd container exists, starting it..."; \
		docker start etcd-gcr-${ETCD_VER} 2>/dev/null || true; \
	else \
		echo "Creating etcd container..."; \
		rm -rf /tmp/etcd-data.tmp && mkdir -p /tmp/etcd-data.tmp && \
			docker rmi gcr.io/etcd-development/etcd:${ETCD_VER} || true && \
			docker run \
			-p 2379:2379 \
			-p 2380:2380 \
			--mount type=bind,source=/tmp/etcd-data.tmp,destination=/etcd-data \
			--name etcd-gcr-${ETCD_VER} \
			gcr.io/etcd-development/etcd:${ETCD_VER} \
			/usr/local/bin/etcd \
			--name s1 \
			--data-dir /etcd-data \
			--listen-client-urls http://0.0.0.0:2379 \
			--advertise-client-urls http://0.0.0.0:2379 \
			--listen-peer-urls http://0.0.0.0:2380 \
			--initial-advertise-peer-urls http://0.0.0.0:2380 \
			--initial-cluster s1=http://0.0.0.0:2380 \
			--initial-cluster-token tkn \
			--initial-cluster-state new \
			--log-level info \
			--logger zap \
			--log-outputs stderr; \
	fi

build-tasks:
	@go build -o bin/cliTasks/cliTasks cmd/cliTasks/main.go

create-tasks: build-tasks
	@./bin/cliTasks/cliTasks

build:
	@go build -o bin/worker/worker cmd/worker/main.go

execute: build
	@./bin/worker/worker

clean:
	@docker stop distributedqueue etcd-gcr-${ETCD_VER} 2>/dev/null || true
	@docker rm distributedqueue etcd-gcr-${ETCD_VER} 2>/dev/null || true
	@rm -rf /tmp/etcd-data.tmp
	@rm -rf ./bin/*