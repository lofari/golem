(ns agents.fix-bug
  (:require [golem.dsl.core :refer [defagent]]
            [golem.dsl.primitives.builtins]
            [golem.dsl.predicates.builtins]))

(defagent fix-bug
  "Diagnoses and fixes a bug."
  {:initial-state [:goal]}

  (research)
  (plan)
  (implement)
  (review)

  (on-error :transient (retry {:max 3})))
