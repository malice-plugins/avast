REPO=malice-plugins/avast
ORG=malice
NAME=avast
VERSION=$(shell cat VERSION)


all: build size test

build:
	docker build -t $(ORG)/$(NAME):$(VERSION) .

size:
	sed -i.bu 's/docker%20image-.*-blue/docker%20image-$(shell docker images --format "{{.Size}}" $(ORG)/$(NAME):$(VERSION)| cut -d' ' -f1)-blue/' README.md

tar: build
	docker save $(ORG)/$(NAME):$(VERSION) -o avast.tar

go-test:
	go get
	go test -v

ssh:
	@docker run --init -it --rm --entrypoint=bash $(ORG)/$(NAME):$(VERSION)

avtest:
	@echo "===> Avast Version"
	@docker run --init --rm --entrypoint=bash $(ORG)/$(NAME):$(VERSION) -c "/etc/init.d/avast start > /dev/null 2>&1 && scan -v" > tests/av_version.out
	@echo "===> Avast VPS"
	@docker run --init --rm --entrypoint=bash $(ORG)/$(NAME):$(VERSION) -c "/etc/init.d/avast start > /dev/null 2>&1 && scan -V" > tests/av_vps.out	
	@echo "===> Avast EICAR Test"
	@docker run --init --rm --entrypoint=bash $(ORG)/$(NAME):$(VERSION) -c "/etc/init.d/avast start > /dev/null 2>&1 && scan -abfu EICAR" > tests/av_scan.out || true

test:
	docker run --rm $(ORG)/$(NAME):$(VERSION) --help
	test -f sample || wget https://github.com/maliceio/malice-av/raw/master/samples/befb88b89c2eb401900a68e9f5b78764203f2b48264fcc3f7121bf04a57fd408 -O sample
	docker run --rm -v $(PWD):/malware $(ORG)/$(NAME):$(VERSION) -t sample > docs/SAMPLE.md
	docker run --rm -v $(PWD):/malware $(ORG)/$(NAME):$(VERSION) -V sample | jq . > docs/results.json
	cat docs/results.json | jq .

circle:
	http https://circleci.com/api/v1.1/project/github/${REPO} | jq '.[0].build_num' > .circleci/build_num
	http "$(shell http https://circleci.com/api/v1.1/project/github/${REPO}/$(shell cat .circleci/build_num)/artifacts${CIRCLE_TOKEN} | jq '.[].url')" > .circleci/SIZE
	sed -i.bu 's/docker%20image-.*-blue/docker%20image-$(shell cat .circleci/SIZE)-blue/' README.md

clean:
	rm sample
	docker-clean stop
	docker rmi $(ORG)/$(NAME)
	docker rmi $(ORG)/$(NAME):$(BUILD)

.PHONY: build size test go-test avtest tar circle clean ssh
