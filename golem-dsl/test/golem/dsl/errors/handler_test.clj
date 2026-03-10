(ns golem.dsl.errors.handler-test
  (:require [clojure.test :refer [deftest is testing]]
            [golem.dsl.errors.handler :as handler]
            [golem.dsl.errors.types :as types]))

(deftest resolves-handler-by-priority
  (testing "per-step override wins"
    (let [step-handlers {:transient {:action :retry :max 5}}
          agent-handlers {:transient {:action :retry :max 3}}
          result (handler/resolve-handler :transient step-handlers agent-handlers)]
      (is (= 5 (:max result)))))

  (testing "agent-level when no step override"
    (let [result (handler/resolve-handler :transient nil {:transient {:action :retry :max 3}})]
      (is (= 3 (:max result)))))

  (testing "global default when nothing else"
    (let [result (handler/resolve-handler :transient nil nil)]
      (is (= :retry (:action result))))))

(deftest classify-error-type
  (is (= :transient (types/classify {:exit-code 1})))
  (is (= :malformed-output (types/classify {:exit-code 0 :output-missing true})))
  (is (= :unrecoverable (types/classify {:spawn-failed true}))))

(deftest retry-logic
  (testing "should retry when under max"
    (is (true? (handler/should-retry? {:action :retry :max 3} 1)))
    (is (true? (handler/should-retry? {:action :re-run :max 2} 0))))
  (testing "should not retry at max"
    (is (false? (handler/should-retry? {:action :retry :max 3} 3)))
    (is (false? (handler/should-retry? {:action :snapshot-and-halt} 0)))))
