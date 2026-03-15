(ns golem.dsl.registry-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [golem.dsl.registry :as registry]
            [golem.dsl.core :refer [defprimitive defagent]]
            [golem.dsl.primitives.builtins]))

(use-fixtures :each (fn [f] (registry/reset-all!)
                            (require 'golem.dsl.primitives.builtins :reload)
                            (f)))

(deftest nodes-returns-agent-nodes
  (defagent test-agent
    {:initial-state [:goal]}
    (plan)
    (implement))
  (let [nodes (registry/nodes :test-agent)]
    (is (= 2 (count nodes)))
    (is (= :plan (-> nodes first :primitive)))))

(deftest edges-returns-agent-edges
  (defagent test-agent
    {:initial-state [:goal]}
    (plan)
    (implement))
  (let [edges (registry/edges :test-agent)]
    (is (seq edges))))

(deftest contracts-returns-full-chain
  (defagent test-agent
    {:initial-state [:goal]}
    (plan)
    (implement))
  (let [contracts (registry/contracts :test-agent)]
    (is (= [:goal] (-> contracts first :reads)))
    (is (= [:code :test-results] (-> contracts second :writes)))))

(deftest validate-returns-nil-for-valid-agent
  (defagent test-agent
    {:initial-state [:goal]}
    (plan)
    (implement))
  (is (nil? (registry/validate :test-agent))))
