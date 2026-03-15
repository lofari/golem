(ns agents.write-docs
  (:require [golem.dsl.core :refer [defagent]]
            [golem.dsl.primitives.builtins]
            [golem.dsl.predicates.builtins]))

(defagent write-docs
  "Writes documentation for existing code."
  {:initial-state [:goal :code :test-results]}

  (reflect)
  (plan)
  (implement)
  (review)

  (on-error :transient (retry {:max 3})))
