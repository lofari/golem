(ns golem.dsl.resolve-test
  (:require [clojure.test :refer :all]
            [golem.dsl.resolve :as resolve]
            [clojure.java.io :as io]))

(deftest resolve-builtin-test
  (let [path (resolve/resolve-agent "build-feature" nil)]
    (is (some? path))
    (is (.contains (str path) "build_feature"))))

(deftest resolve-project-local-test
  (let [dir (str (System/getProperty "java.io.tmpdir") "/resolve-test-" (System/currentTimeMillis))
        agents-dir (str dir "/agents")
        _ (.mkdirs (io/file agents-dir))
        _ (spit (str agents-dir "/my-flow.clj") "(defagent my-flow)")
        path (resolve/resolve-agent "my-flow" agents-dir)]
    (is (some? path))
    (is (.contains (str path) "my-flow.clj"))
    ;; cleanup
    (.delete (io/file (str agents-dir "/my-flow.clj")))
    (.delete (io/file agents-dir))
    (.delete (io/file dir))))

(deftest resolve-project-overrides-builtin-test
  (let [dir (str (System/getProperty "java.io.tmpdir") "/resolve-override-" (System/currentTimeMillis))
        agents-dir (str dir "/agents")
        _ (.mkdirs (io/file agents-dir))
        _ (spit (str agents-dir "/build-feature.clj") "(defagent build-feature :custom)")
        path (resolve/resolve-agent "build-feature" agents-dir)]
    (is (.contains (str path) agents-dir))
    ;; cleanup
    (.delete (io/file (str agents-dir "/build-feature.clj")))
    (.delete (io/file agents-dir))
    (.delete (io/file dir))))

(deftest resolve-unknown-returns-nil-test
  (is (nil? (resolve/resolve-agent "nonexistent-agent" nil))))

(deftest list-agents-test
  (let [agents (resolve/list-agents nil)]
    (is (>= (count agents) 4))
    (is (some #(= (:name %) "build-feature") agents))))
