(ns golem.dsl.primitives.builtins-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [golem.dsl.primitives.builtins]
            [golem.dsl.predicates.builtins]
            [golem.dsl.registry :as registry]))

(use-fixtures :each (fn [f]
                      (registry/reset-all!)
                      ;; Re-require to re-register builtins after reset
                      (require 'golem.dsl.primitives.builtins :reload)
                      (require 'golem.dsl.predicates.builtins :reload)
                      (f)))

(deftest all-builtins-registered
  (is (some? (registry/get-primitive :plan)))
  (is (some? (registry/get-primitive :implement)))
  (is (some? (registry/get-primitive :review)))
  (is (some? (registry/get-primitive :reflect)))
  (is (some? (registry/get-primitive :research)))
  (is (some? (registry/get-primitive :run-tests))))

(deftest all-predicates-registered
  (is (some? (registry/get-predicate :failed?)))
  (is (some? (registry/get-predicate :needs-work?))))

(deftest predicates-evaluate-correctly
  (let [failed-fn (registry/get-predicate :failed?)]
    (is (true? (failed-fn {:test-results {:status :fail}})))
    (is (false? (failed-fn {:test-results {:status :pass}}))))
  (let [needs-work-fn (registry/get-predicate :needs-work?)]
    (is (true? (needs-work-fn {:review-feedback {:verdict :needs-work}})))
    (is (false? (needs-work-fn {:review-feedback {:verdict :approved}})))))
