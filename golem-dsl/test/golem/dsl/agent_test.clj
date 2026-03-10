(ns golem.dsl.agent-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [golem.dsl.core :refer [defagent defprimitive defpred]]
            [golem.dsl.registry :as registry]))

(use-fixtures :each (fn [f] (registry/reset-all!) (f)))

(defn- setup-primitives! []
  (defprimitive plan
    "Plans from goal."
    {:reads [:goal] :writes [:plan] :session true}
    (fn [state context adapter] {:plan [{:step 1 :desc "do thing"}]}))
  (defprimitive implement
    "Writes code."
    {:reads [:goal :plan] :optional-reads [:review-feedback]
     :writes [:code :test-results] :session true}
    (fn [state context adapter] {:code {:files ["a.go"]} :test-results {:status :pass}}))
  (defprimitive review
    "Reviews code."
    {:reads [:code :test-results] :writes [:review-feedback] :session true}
    (fn [state context adapter] {:review-feedback {:verdict :approved}}))
  (defpred needs-work?
    (= :needs-work (get-in state [:review-feedback :verdict]))))

(deftest defagent-registers-graph
  (setup-primitives!)
  (defagent build-feature
    {:initial-state [:goal]}
    (plan)
    (implement)
    (review))
  (let [agent (registry/get-agent :build-feature)]
    (is (some? agent))
    (is (= 3 (count (:nodes agent))))
    (is (= :plan (-> agent :nodes first :primitive)))))

(deftest defagent-rejects-unsatisfied-reads
  (setup-primitives!)
  (is (thrown? Exception
    (eval '(do
      (require '[golem.dsl.core :refer [defagent]])
      (golem.dsl.core/defagent bad-agent
        {:initial-state [:goal]}
        (implement)))))))
