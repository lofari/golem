(ns golem.dsl.integration-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [golem.dsl.core :refer [defagent defprimitive defpred]]
            [golem.dsl.engine.core :as engine]
            [golem.dsl.engine.snapshot :as snapshot]
            [golem.dsl.session.protocol :as proto]
            [golem.dsl.registry :as registry]
            [clojure.java.io :as io]))

(use-fixtures :each (fn [f] (registry/reset-all!) (f)))

(defrecord MockAdapter []
  proto/SessionAdapter
  (spawn [_ prompt working-dir opts] {:prompt prompt :dir working-dir})
  (await-result [_ handle timeout-ms] {:exit-code 0})
  (read-output [_ handle] {}))

(deftest full-agent-lifecycle
  (let [base-dir (str (System/getProperty "java.io.tmpdir")
                      "/golem-integration-" (System/currentTimeMillis))
        adapter (->MockAdapter)]

    ;; Register primitives with deterministic output (non-session for testing)
    (defprimitive plan
      "Plans." {:reads [:goal] :writes [:plan] :session false}
      (fn [s c a] {:plan [{:step 1 :desc "implement it"}]}))
    (defprimitive implement
      "Implements." {:reads [:goal :plan] :writes [:code :test-results] :session false}
      (fn [s c a] {:code {:files ["main.go"] :language "go"}
                   :test-results {:status :pass :failures []}}))

    (defagent integration-test-agent
      {:initial-state [:goal]}
      (plan)
      (implement))

    (let [result (engine/run :integration-test-agent
                             {:goal "Build a thing"}
                             {:adapter adapter
                              :base-dir base-dir})]
      ;; Verify final state
      (is (= :pass (get-in result [:state :test-results :status])))
      (is (= 2 (:version result)))
      (is (= :completed (:status result)))

      ;; Verify snapshots exist
      (is (some? (snapshot/load-state base-dir (:run-id result) 0)))
      (is (some? (snapshot/load-state base-dir (:run-id result) 1)))
      (is (some? (snapshot/load-state base-dir (:run-id result) 2))))))
