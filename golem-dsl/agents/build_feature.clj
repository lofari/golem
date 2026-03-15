(ns agents.build-feature
  (:require [golem.dsl.core :refer [defagent]]
            [golem.dsl.primitives.builtins]
            [golem.dsl.predicates.builtins]))

(defagent build-feature
  "Builds a feature from a goal description."
  {:initial-state [:goal]}

  (plan)
  (implement)
  (review)
  (while needs-work? {:max 3}
    (implement)
    (review))

  (on-error :transient        (retry {:max 3}))
  (on-error :malformed-output (re-run {:hint "Write session-output.edn with contract keys."}))
  (on-error :contract-violation (snapshot-and-halt)))
