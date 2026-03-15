(ns golem.dsl.engine.context-test
  (:require [clojure.test :refer [deftest is testing]]
            [golem.dsl.engine.context :as context]))

(deftest renders-template-with-state-keys
  (let [state {:goal "Build CSV converter"
               :plan [{:step 1 :desc "Parse args"}]}
        contract {:reads [:goal :plan]
                  :optional-reads [:reflection :review-feedback]}
        result (context/render-prompt :implement state contract)]
    (is (string? result))
    (is (.contains result "Build CSV converter"))
    (is (.contains result "Parse args"))))

(deftest skips-missing-optional-reads
  (let [state {:goal "Test goal" :plan [{:step 1 :desc "do it"}]}
        contract {:reads [:goal :plan]
                  :optional-reads [:reflection :review-feedback]}
        result (context/render-prompt :implement state contract)]
    (is (not (.contains result "Previous Reflection")))))
