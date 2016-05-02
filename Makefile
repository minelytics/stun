test:
	go test -v
bench:
	go test -bench .
fuzz-prepare-msg:
	go-fuzz-build -func FuzzMessage -o stun-msg-fuzz.zip github.com/cydev/stun
fuzz-prepare-typ:
	go-fuzz-build -func FuzzType -o stun-typ-fuzz.zip github.com/cydev/stun
fuzz-msg:
	go-fuzz -bin=./stun-msg-fuzz.zip -workdir=examples/stun-msg
fuzz-typ:
	go-fuzz -bin=./stun-typ-fuzz.zip -workdir=examples/stun-typ
lint:
	@gometalinter -e "AttrType.+gocyclo" \
		-e "_test.go.+(gocyclo|errcheck|dupl)" \
		--enable="lll" --line-length=80 \
		--disable=gotype
escape:
	@echo "Not escapes, except autogenerated:"
	@go build -gcflags '-m -l' 2>&1 \
	| grep escape \
	| grep -v "<autogenerated>" \
	| grep -v escapes

blackbox:
	@TEST_EXTERNAL=1 go test -run TestClientSend -v
format:
	goimports -w .
bench-compare:
	go test -bench . > bench.go-16
	go-tip test -bench . > bench.go-tip
	@benchcmp bench.go-16 bench.go-tip

install:
	go get -u sourcegraph.com/sqs/goreturns
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install --update
	go get -u github.com/cydev/go-fuzz/go-fuzz-build
	go get -u github.com/dvyukov/go-fuzz/go-fuzz
