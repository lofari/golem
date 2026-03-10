.PHONY: build test native clean

CLJ = clojure

test:
	$(CLJ) -M:test

build:
	$(CLJ) -M:run --help

native: target/golem-dsl
	@echo "Built target/golem-dsl"

target/golem-dsl: $(shell find src -name '*.clj') deps.edn
	mkdir -p target
	$(CLJ) -M:native-image -e "(compile 'golem.dsl.cli.main)"
	native-image \
		--no-fallback \
		--initialize-at-build-time \
		-H:ReflectionConfigurationFiles=graalvm/reflect-config.json \
		-jar target/golem-dsl.jar \
		-o target/golem-dsl

clean:
	rm -rf target .cpcache
