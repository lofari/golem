(ns golem.dsl.cli.main-test
  (:require [clojure.test :refer :all]
            [golem.dsl.cli.main :as cli]))

(deftest parse-args-run-test
  (let [parsed (cli/parse-args ["run" "build-feature" "--goal" "add auth" "--state-dir" "/tmp/test"])]
    (is (= :run (:command parsed)))
    (is (= "build-feature" (:agent parsed)))
    (is (= "add auth" (:goal parsed)))
    (is (= "/tmp/test" (:state-dir parsed)))))

(deftest parse-args-opts-test
  (let [parsed (cli/parse-args ["run" "fix-bug" "--goal" "fix" "--state-dir" "." "--opt" "max_iterations=3"])]
    (is (= {"max_iterations" "3"} (:opts parsed)))))

(deftest parse-args-list-test
  (let [parsed (cli/parse-args ["list"])]
    (is (= :list (:command parsed)))))

(deftest parse-args-no-command-test
  (let [parsed (cli/parse-args [])]
    (is (= :help (:command parsed)))))

(deftest parse-args-dry-run-test
  (let [parsed (cli/parse-args ["run" "build-feature" "--goal" "test" "--state-dir" "." "--dry-run"])]
    (is (:dry-run parsed))))
