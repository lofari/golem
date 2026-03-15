(ns golem.dsl.engine.core-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [golem.dsl.engine.core :as engine]
            [golem.dsl.engine.snapshot :as snapshot]
            [golem.dsl.session.protocol :as proto]
            [golem.dsl.core :refer [defprimitive defpred defagent]]
            [golem.dsl.registry :as registry]))

(use-fixtures :each (fn [f] (registry/reset-all!) (f)))

;; Mock adapter — primitives use :session false so this is just a placeholder
(defrecord MockAdapter []
  proto/SessionAdapter
  (spawn [_ prompt working-dir opts] {:prompt prompt :dir working-dir})
  (await-result [_ handle timeout-ms] {:exit-code 0})
  (read-output [_ handle] {}))

(deftest engine-runs-sequential-agent
  (let [base-dir (str (System/getProperty "java.io.tmpdir")
                      "/golem-engine-test-" (System/currentTimeMillis))
        adapter (->MockAdapter)]
    ;; Register primitives that execute locally (no session)
    (defprimitive plan
      "Plans." {:reads [:goal] :writes [:plan] :session false}
      (fn [state context adapter]
        {:plan [{:step 1 :desc "do thing"}]}))
    (defprimitive implement
      "Implements." {:reads [:goal :plan] :writes [:code :test-results] :session false}
      (fn [state context adapter]
        {:code {:files ["a.go"]} :test-results {:status :pass :failures []}}))

    (defagent simple-agent
      {:initial-state [:goal]}
      (plan)
      (implement))

    (let [result (engine/run :simple-agent
                             {:goal "Build a thing"}
                             {:adapter adapter
                              :base-dir base-dir})]
      (is (= :completed (:status result)))
      (is (= :pass (get-in result [:state :test-results :status])))
      (is (= 2 (:version result)))
      ;; Verify snapshots exist
      (is (some? (snapshot/load-state base-dir (:run-id result) 0)))
      (is (some? (snapshot/load-state base-dir (:run-id result) 1)))
      (is (some? (snapshot/load-state base-dir (:run-id result) 2))))))

(deftest engine-halts-on-unrecoverable-error
  (let [base-dir (str (System/getProperty "java.io.tmpdir")
                      "/golem-engine-err-" (System/currentTimeMillis))
        adapter (->MockAdapter)]
    ;; A primitive that requires :plan but we won't provide it
    (defprimitive needs-plan
      "Needs plan." {:reads [:goal :plan] :writes [:code] :session false}
      (fn [state context adapter] {:code {}}))

    (defagent bad-agent
      {:initial-state [:goal :plan]}
      (needs-plan))

    ;; Run with missing :plan key
    (let [result (engine/run :bad-agent
                             {:goal "Build a thing"}
                             {:adapter adapter
                              :base-dir base-dir})]
      (is (= :halted (:status result))))))
