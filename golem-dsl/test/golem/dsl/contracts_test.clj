(ns golem.dsl.contracts-test
  (:require [clojure.test :refer [deftest is testing]]
            [golem.dsl.contracts :as contracts]))

(deftest validate-reads-satisfied
  (testing "all reads satisfied by initial-state + prior writes"
    (let [steps [{:id :plan
                  :primitive :plan
                  :contract {:reads [:goal] :writes [:plan]}}
                 {:id :implement-1
                  :primitive :implement
                  :contract {:reads [:goal :plan] :writes [:code :test-results]}}]
          initial-state [:goal]]
      (is (nil? (contracts/validate-chain steps initial-state)))))

  (testing "unsatisfied read throws"
    (let [steps [{:id :implement-1
                  :primitive :implement
                  :contract {:reads [:goal :plan] :writes [:code :test-results]}}]
          initial-state [:goal]]
      (let [errors (contracts/validate-chain steps initial-state)]
        (is (some? errors))
        (is (= :plan (-> errors first :missing-key)))))))

(deftest validate-optional-reads-not-required
  (testing "optional-reads don't fail validation"
    (let [steps [{:id :plan
                  :primitive :plan
                  :contract {:reads [:goal]
                             :optional-reads [:reflection]
                             :writes [:plan]}}]
          initial-state [:goal]]
      (is (nil? (contracts/validate-chain steps initial-state))))))
