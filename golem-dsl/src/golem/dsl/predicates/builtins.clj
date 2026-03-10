(ns golem.dsl.predicates.builtins
  (:require [golem.dsl.core :refer [defpred]]))

(defpred failed?
  (= :fail (get-in state [:test-results :status])))

(defpred needs-work?
  (= :needs-work (get-in state [:review-feedback :verdict])))
